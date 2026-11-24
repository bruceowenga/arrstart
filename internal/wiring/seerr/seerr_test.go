package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeServer simulates Seerr's v1 API surface, mirroring seerr/seerr:v3.5.2
// as verified against the real container. Paths under test:
//
//	GET /api/v1/status            anonymous 200
//	GET /api/v1/settings/jellyfin 200 if initialized, else 403
//	POST /api/v1/settings/{jellyfin,sonarr,radarr} 204 if initialized,
//	                                                    else 403
//
// Seerr v3.5.2 blocks every settings write with 403 until the instance
// has been initialized through the web UI at /setup; there is no
// headless login route. The fake models both states.
type fakeServer struct {
	initialized bool
	writes      []string // settings paths POSTed, in order
	bodies      map[string][]byte
}

func newFakeServer(initialized bool) *fakeServer {
	return &fakeServer{
		initialized: initialized,
		bodies:      map[string][]byte{},
	}
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	p := r.URL.Path
	switch {
	case p == "/api/v1/status" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"status":"online"}`)
		return
	case p == "/api/v1/settings/jellyfin" && r.Method == http.MethodGet:
		if !f.initialized {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":"You do not have permission to access this endpoint"}`)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"name":"Jellyfin","active":true}`)
		return
	case strings.HasPrefix(p, "/api/v1/settings/") && r.Method == http.MethodPost:
		if !f.initialized {
			w.WriteHeader(http.StatusForbidden)
			_, _ = fmt.Fprint(w, `{"error":"You do not have permission to access this endpoint"}`)
			return
		}
		body, _ := io.ReadAll(io.LimitReader(r.Body, 512))
		f.writes = append(f.writes, p)
		f.bodies[p] = body
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

func TestWaitHealthy(t *testing.T) {
	srv := httptest.NewServer(newFakeServer(true))
	defer srv.Close()
	c := New(srv.URL, "test-key")
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
	c := New(srv.URL, "test-key")
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.WaitHealthy(ctx); err != context.Canceled {
		t.Fatalf("WaitHealthy on down server with cancelled ctx: got %v, want context.Canceled", err)
	}
}

// TestConfigureNotInitialized exercises Configure against a Seerr instance
// that has not been initialized via /setup — settings writes return 403 and
// Configure must surface a clear operator-directed error.
func TestConfigureNotInitialized(t *testing.T) {
	f := newFakeServer(false)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")

	conf := &Config{
		APIKey:   "seerr-api-key",
		Jellyfin: AppConnection{URL: "http://jellyfin:8096", APIKey: "jf-key"},
		Sonarr:   AppConnection{URL: "http://sonarr:8989", APIKey: "sonarr-key"},
		Radarr:   AppConnection{URL: "http://radarr:7878", APIKey: "radarr-key"},
	}
	if err := c.Configure(newCtx(t), conf); err == nil {
		t.Fatal("Configure: expected error for uninitialized instance")
	} else if !strings.Contains(err.Error(), "/setup") {
		t.Fatalf("Configure: error should direct to /setup, got %v", err)
	}
	if len(f.writes) != 0 {
		t.Fatalf("Configure: expected no settings writes before initialization, got %v", f.writes)
	}
}

// TestConfigureInitialized exercises Configure against an initialized
// instance where every settings endpoint accepts the payload (204).
func TestConfigureInitialized(t *testing.T) {
	f := newFakeServer(true)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")

	conf := &Config{
		APIKey:   "seerr-api-key",
		Jellyfin: AppConnection{URL: "http://jellyfin:8096", APIKey: "jf-key"},
		Sonarr:   AppConnection{URL: "http://sonarr:8989", APIKey: "sonarr-key"},
		Radarr:   AppConnection{URL: "http://radarr:7878", APIKey: "radarr-key"},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	want := []string{
		"/api/v1/settings/jellyfin",
		"/api/v1/settings/sonarr",
		"/api/v1/settings/radarr",
	}
	if !sliceContainsStr(f.writes, want[0]) || !sliceContainsStr(f.writes, want[1]) || !sliceContainsStr(f.writes, want[2]) {
		t.Fatalf("Configure: expected all three settings writes, got %v", f.writes)
	}
	for _, body := range f.bodies {
		var decoded map[string]any
		if err := json.Unmarshal(body, &decoded); err != nil {
			t.Fatalf("Configure: settings body not valid JSON: %v", err)
		}
	}
}

// TestConfigureIdempotent re-runs Configure and expects no duplicate
// server-observable side effects beyond the allowed writes.
func TestConfigureIdempotent(t *testing.T) {
	f := newFakeServer(true)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")

	conf := &Config{
		APIKey:   "seerr-api-key",
		Jellyfin: AppConnection{URL: "http://jellyfin:8096", APIKey: "jf-key"},
		Sonarr:   AppConnection{URL: "http://sonarr:8989", APIKey: "sonarr-key"},
		Radarr:   AppConnection{URL: "http://radarr:7878", APIKey: "radarr-key"},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure #1: %v", err)
	}
	writes1 := len(f.writes)
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure #2 (idempotent re-run): %v", err)
	}
	if len(f.writes) != writes1*2 {
		t.Fatalf("Configure re-run: expected %d writes, got %d", writes1*2, len(f.writes))
	}
}

// TestVerifyInitialized checks Verify against an initialized instance.
func TestVerifyInitialized(t *testing.T) {
	f := newFakeServer(true)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")
	if err := c.Verify(newCtx(t)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

// TestVerifyNotInitialized checks Verify reports the uninitialized state
// without hard failure semantics beyond a clear error.
func TestVerifyNotInitialized(t *testing.T) {
	f := newFakeServer(false)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")
	if err := c.Verify(newCtx(t)); err == nil {
		t.Fatal("Verify: expected not-initialized error")
	} else if !strings.Contains(err.Error(), "/setup") {
		t.Fatalf("Verify: error should direct to /setup, got %v", err)
	}
}

// TestConfigureEmptyConfig skips connections with empty URLs and returns nil
// once reachability passes.
func TestConfigureEmptyConfig(t *testing.T) {
	f := newFakeServer(true)
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL, "test-key")
	conf := &Config{APIKey: "seerr-api-key"}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure with empty connections: %v", err)
	}
}

// newCtx returns a context with a short timeout.
func newCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func sliceContainsStr(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}
