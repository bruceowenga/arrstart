package prowlarr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

type fakeProwlarrServer struct {
	*httptest.Server
	apiKey       string
	applications []existingApplication
	postAppCalls int
	lastApiKey   string
	requireKey   bool
}

type postAppRequest struct {
	Name           string `json:"name"`
	Implementation string `json:"implementation"`
	ConfigContract string `json:"configContract"`
}

func newFakeProwlarrServer(t *testing.T, requireKey bool) *fakeProwlarrServer {
	t.Helper()
	f := &fakeProwlarrServer{apiKey: "prowlarr-test-key", requireKey: requireKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v1/system/status", f.handleStatus)
	mux.HandleFunc("/api/v1/applications", f.handleApplications)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeProwlarrServer) requireKeyOK(r *http.Request) bool {
	if !f.requireKey {
		return true
	}
	f.lastApiKey = r.Header.Get("X-Api-Key")
	return f.lastApiKey == f.apiKey
}

func (f *fakeProwlarrServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var _ = json.NewEncoder(w).Encode(map[string]string{"version": "3.0.2"})
}

func (f *fakeProwlarrServer) handleApplications(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		var _ = json.NewEncoder(w).Encode(f.applications)
	case http.MethodPost:
		f.postAppCalls++
		var req postAppRequest
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &req); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		for _, a := range f.applications {
			if a.Name == req.Name {
				w.Header().Set("Content-Type", "application/json")
				w.WriteHeader(http.StatusBadRequest)
				var _ = json.NewEncoder(w).Encode([]map[string]string{
					{"propertyName": "Name", "errorMessage": "Should be unique", "attemptedValue": req.Name},
				})
				return
			}
		}
		a := existingApplication{Name: req.Name, ID: len(f.applications) + 1, Enable: true}
		f.applications = append(f.applications, a)
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func TestConfigureRegistersApplications(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, srv.apiKey)
	cfg := &Config{
		APIKey: srv.apiKey,
		Apps: []App{
			{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings",
				BaseURL: "http://sonarr:8989", APIKey: "sonarr-key"},
			{Name: "Radarr", Implementation: "Radarr", ConfigContract: "RadarrSettings",
				BaseURL: "http://radarr:7878", APIKey: "radarr-key"},
		},
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.postAppCalls != 2 {
		t.Errorf("expected 2 add application calls, got %d", srv.postAppCalls)
	}
	last := srv.applications[len(srv.applications)-1]
	if last.Name != "Radarr" {
		t.Errorf("last application = %q, want Radarr", last.Name)
	}
	if srv.lastApiKey != srv.apiKey {
		t.Errorf("X-Api-Key = %q, want %q", srv.lastApiKey, srv.apiKey)
	}
}

func TestConfigureIsIdempotent(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()

	srv.applications = []existingApplication{{Name: "Sonarr", ID: 1, Enable: true}}
	c := New(srv.URL, srv.apiKey)
	cfg := &Config{
		APIKey: srv.apiKey,
		Apps: []App{
			{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings"},
			{Name: "Radarr", Implementation: "Radarr", ConfigContract: "RadarrSettings"},
		},
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.postAppCalls != 1 {
		t.Errorf("expected 1 add application call (Sonarr already present), got %d", srv.postAppCalls)
	}
	added := srv.applications[len(srv.applications)-1]
	if added.Name != "Radarr" {
		t.Errorf("added application = %q, want Radarr", added.Name)
	}
}

func TestConfigureRejectsInvalidConfig(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.Configure(context.Background(), "not-a-config"); err == nil {
		t.Fatal("expected error for non-*Config")
	}
}

func TestVerifySuccessAndFailure(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()

	srv.applications = []existingApplication{
		{Name: "Sonarr", ID: 1, Enable: true},
		{Name: "Radarr", ID: 2, Enable: true},
	}
	c := New(srv.URL, srv.apiKey)
	c.apps = []App{
		{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings"},
		{Name: "Radarr", Implementation: "Radarr", ConfigContract: "RadarrSettings"},
	}
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Radarr missing fails.
	srv.applications = []existingApplication{{Name: "Sonarr", ID: 1, Enable: true}}
	c.apps = []App{
		{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings"},
		{Name: "Radarr", Implementation: "Radarr", ConfigContract: "RadarrSettings"},
	}
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for missing Radarr application")
	} else if !strings.Contains(err.Error(), "Radarr") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifyWrongAPIKey(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "wrong-key")
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for wrong API key")
	}
}

func TestWaitHealthyLiveAndDead(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.WaitHealthy(context.Background()); err != nil {
		t.Fatalf("WaitHealthy on live server: %v", err)
	}

	down := New("http://127.0.0.1:1", srv.apiKey)
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	if err := down.WaitHealthy(ctx); err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestConfigureWithoutKeyAgainstKeyServerFails(t *testing.T) {
	srv := newFakeProwlarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "")
	cfg := &Config{Apps: []App{{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings"}}}
	if err := c.Configure(context.Background(), cfg); err == nil {
		t.Fatal("expected error when no API key against key-requiring server")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("http://127.0.0.1:9696/", "key")
	if c.baseURL != "http://127.0.0.1:9696" {
		t.Errorf("baseURL = %q, expected trailing slash trimmed", c.baseURL)
	}
}
