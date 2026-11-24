package seerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeSeerrServer simulates Seerr's /api/v1 settings surface, mirroring the
// real pinned container seerr/seerr:v3.5.2 as verified in docs/spikes/
// 04-seerr-setup.md:
//
//	GET    /api/v1/status              200 {"version":"3.5.2"}
//	POST   /api/v1/settings/jellyfin   200 when initialized, 403 otherwise
//	GET    /api/v1/settings/jellyfin   returns stored settings (403 otherwise)
//	POST   /api/v1/settings/sonarr     201 when initialized, 403 otherwise
//	GET    /api/v1/settings/sonarr     returns stored settings (403 otherwise)
//	POST   /api/v1/settings/radarr     201 when initialized, 403 otherwise
//	GET    /api/v1/settings/radarr     returns stored settings (403 otherwise)
//
// settings writes require X-Api-Key; when initialized=false they return 403
// (verified live: settings writes are permission-blocked until the admin user
// is created once through the web UI at /setup).
type fakeSeerrServer struct {
	*httptest.Server
	initialized bool
	apiKey      string
	requireKey  bool

	jellyfin *settingsEntry
	sonarr   *settingsEntry
	radarr   *settingsEntry
}

type settingsEntry struct {
	statusCode int
	body       []byte // last request body stored
}

func newFakeSeerrServer(t *testing.T, initialized bool, requireKey bool) *fakeSeerrServer {
	t.Helper()
	f := &fakeSeerrServer{initialized: initialized, apiKey: "seerr-test-key", requireKey: requireKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/status", f.handleStatus)
	mux.HandleFunc("/api/v1/settings/jellyfin", f.handleSettings(&f.jellyfin, http.StatusOK))
	mux.HandleFunc("/api/v1/settings/sonarr", f.handleSettings(&f.sonarr, http.StatusCreated))
	mux.HandleFunc("/api/v1/settings/radarr", f.handleSettings(&f.radarr, http.StatusCreated))
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeSeerrServer) keyOK(r *http.Request) bool {
	if !f.requireKey {
		return true
	}
	return r.Header.Get("X-Api-Key") == f.apiKey
}

func (f *fakeSeerrServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodGet {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	_, _ = fmt.Fprint(w, `{"version":"3.5.2","commitTag":"x","updateAvailable":false,"commitsBehind":0,"restartRequired":false}`)
}

func (f *fakeSeerrServer) handleSettings(entry **settingsEntry, success int) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		if !f.keyOK(r) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if !f.initialized {
			http.Error(w, `{"status":403,"error":"You do not have permission to access this endpoint"}`, http.StatusForbidden)
			return
		}
		switch r.Method {
		case http.MethodPost:
			body, _ := io.ReadAll(r.Body)
			*entry = &settingsEntry{statusCode: success, body: body}
			w.WriteHeader(success)
		default:
			// GET returns the last stored settings (mirroring Seerr echoing back
			// what was persisted). Real Seerr answers GET with 200 regardless of
			// the POST creation status (201).
			if *entry != nil && len((*entry).body) > 0 {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusOK)
				_, _ = w.Write((*entry).body)
			} else {
				http.NotFound(w, r)
			}
		}
	}
}

// stored returns the decoded map of the last settings body for entry, or nil.
func (f *fakeSeerrServer) stored(entry *settingsEntry) map[string]any {
	if entry == nil || len(entry.body) == 0 {
		return nil
	}
	var m map[string]any
	if err := json.Unmarshal(entry.body, &m); err != nil {
		return nil
	}
	return m
}

func TestWaitHealthy(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.WaitHealthy(newCtx(t)); err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}
}

func TestWaitHealthyDown(t *testing.T) {
	done := make(chan struct{})
	defer close(done)
	srv := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {}))
	go func() {
		<-done
		srv.Start()
	}()
	time.Sleep(50 * time.Millisecond)
	c := New(srv.URL, "key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.WaitHealthy(ctx); err != context.Canceled {
		t.Fatalf("WaitHealthy on down server with cancelled ctx: got %v, want context.Canceled", err)
	}
}

func TestConfigureSuccessInitialized(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)

	cfg := &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096", Username: "admin", Password: "passw0rd"},
		Sonarr:   SonarrConn{URL: "http://sonarr:8989", APIKey: "sonarr-key", QualityProfileID: 1, RootFolder: "/data/media/tv", LanguageProfileID: 1},
		Radarr:   RadarrConn{URL: "http://radarr:7878", APIKey: "radarr-key", QualityProfileID: 1, RootFolder: "/data/media/movies", MinimumAvailability: "In Cinema"},
	}
	if err := c.Configure(newCtx(t), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.configured == nil {
		t.Fatal("Configure: config not remembered")
	}

	j := srv.stored(srv.jellyfin)
	if j == nil || j["hostname"] != "http://jellyfin:8096" || j["adminUser"] != "admin" {
		t.Fatalf("Configure: Jellyfin settings not stored: %v", srv.stored(srv.jellyfin))
	}
	so := srv.stored(srv.sonarr)
	if so == nil || so["hostname"] != "http://sonarr:8989" || so["activeProfileId"] != float64(1) || so["activeDirectory"] != "/data/media/tv" {
		t.Fatalf("Configure: Sonarr settings not stored: %v", srv.stored(srv.sonarr))
	}
	ra := srv.stored(srv.radarr)
	if ra == nil || ra["hostname"] != "http://radarr:7878" || ra["activeProfileId"] != float64(1) || ra["minimumAvailability"] != "In Cinema" {
		t.Fatalf("Configure: Radarr settings not stored: %v", srv.stored(srv.radarr))
	}
}

func TestConfigureNotInitialized(t *testing.T) {
	srv := newFakeSeerrServer(t, false, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	cfg := &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"},
	}
	err := c.Configure(newCtx(t), cfg)
	if err == nil {
		t.Fatal("Configure against uninitialized Seerr: expected error")
	}
	if !errors.Is(err, errSeerrNotInitialized) {
		t.Errorf("Configure: err is not errSeerrNotInitialized: %v", err)
	}
	if !strings.Contains(err.Error(), "/setup") || !strings.Contains(err.Error(), "rewire") {
		t.Errorf("Configure: error missing setup/rewire guidance: %v", err)
	}
	if c.configured != nil {
		t.Fatal("Configure: config remembered despite failure")
	}
}

func TestConfigureNoKeyAgainstKeyServer(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, "")
	cfg := &Config{BaseURL: srv.URL, Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"}}
	if err := c.Configure(newCtx(t), cfg); err == nil {
		t.Fatal("Configure without API key against key-requiring server: expected error")
	}
}

func TestConfigureEmptyBaseURL(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.Configure(newCtx(t), &Config{}); err == nil {
		t.Fatal("Configure with empty BaseURL: expected error")
	}
}

func TestConfigureRejectsInvalidConfig(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.Configure(newCtx(t), "not-a-config"); err == nil {
		t.Fatal("expected error for non-*Config")
	}
}

func TestVerifySuccess(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)

	conf := &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"},
		Sonarr:   SonarrConn{URL: "http://sonarr:8989", QualityProfileID: 1, RootFolder: "/data/media/tv"},
		Radarr:   RadarrConn{URL: "http://radarr:7878", QualityProfileID: 1, RootFolder: "/data/media/movies"},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// A fresh client representing a later process, feeding only stored state.
	c2 := New(srv.URL, srv.apiKey)
	c2.configured = conf
	if err := c2.Verify(newCtx(t)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyMismatch(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)

	conf := &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Fake now reports a different hostname, so Verify must fail.
	srv.jellyfin = &settingsEntry{
		statusCode: http.StatusOK,
		body:       []byte(`{"hostname":"http://other:9999"}`),
	}
	c2 := New(srv.URL, srv.apiKey)
	c2.configured = conf
	if err := c2.Verify(newCtx(t)); err == nil {
		t.Fatal("Verify: expected mismatch error")
	} else if !strings.Contains(err.Error(), "http://jellyfin:8096") {
		t.Errorf("Verify: wrong error: %v", err)
	}
}

func TestVerifyNotInitialized(t *testing.T) {
	srv := newFakeSeerrServer(t, false, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	c.configured = &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"},
	}
	err := c.Verify(newCtx(t))
	if err == nil {
		t.Fatal("Verify against uninitialized Seerr: expected error")
	}
	if !errors.Is(err, errSeerrNotInitialized) {
		t.Errorf("Verify: err is not errSeerrNotInitialized: %v", err)
	}
}

func TestConfigureSkipsUnconfiguredConnections(t *testing.T) {
	srv := newFakeSeerrServer(t, true, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	// Only Jellyfin configured; Sonarr/Radarr empty URLs must be skipped.
	cfg := &Config{
		BaseURL:  srv.URL,
		APIKey:   srv.apiKey,
		Jellyfin: JellyfinConn{URL: "http://jellyfin:8096"},
	}
	if err := c.Configure(newCtx(t), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.jellyfin == nil {
		t.Fatal("Configure: Jellyfin settings not stored")
	}
	if srv.sonarr != nil || srv.radarr != nil {
		t.Fatalf("Configure: unconfigured connections were written: sonarr=%v radarr=%v", srv.sonarr, srv.radarr)
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("http://127.0.0.1:5055/", "key")
	if c.baseURL != "http://127.0.0.1:5055" {
		t.Errorf("baseURL = %q, expected trailing slash trimmed", c.baseURL)
	}
}

// newCtx returns a context with a short timeout.
func newCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}
