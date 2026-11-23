package radarr

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

type fakeRadarrServer struct {
	*httptest.Server
	apiKey            string
	downloadClients   []servarrDLClient
	rootFolders       []servarrRootFolder
	settings          map[string]any
	addClientCalls    int
	addFolderCalls    int
	postSettingsCalls int
	lastApiKey        string
	requireKey        bool
}

type servarrDLClient struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Enable   bool   `json:"enable"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Category string `json:"category"`
}
type servarrRootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

func newFakeRadarrServer(t *testing.T, requireKey bool) *fakeRadarrServer {
	t.Helper()
	f := &fakeRadarrServer{apiKey: "radarr-test-key", requireKey: requireKey}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/system/status", f.handleStatus)
	mux.HandleFunc("/api/v3/downloadclient", f.handleDownloadClient)
	mux.HandleFunc("/api/v3/rootfolder", f.handleRootFolder)
	mux.HandleFunc("/api/v3/settings", f.handleSettings)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeRadarrServer) requireKeyOK(r *http.Request) bool {
	if !f.requireKey {
		return true
	}
	f.lastApiKey = r.Header.Get("X-Api-Key")
	return f.lastApiKey == f.apiKey
}

func (f *fakeRadarrServer) handleStatus(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	var _ = json.NewEncoder(w).Encode(map[string]string{"version": "7.0.0"})
}

func (f *fakeRadarrServer) handleDownloadClient(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		var _ = json.NewEncoder(w).Encode(f.downloadClients)
	case http.MethodPost:
		f.addClientCalls++
		var dc servarrDLClient
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &dc); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		dc.ID = len(f.downloadClients) + 1
		f.downloadClients = append(f.downloadClients, dc)
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeRadarrServer) handleRootFolder(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		var _ = json.NewEncoder(w).Encode(f.rootFolders)
	case http.MethodPost:
		f.addFolderCalls++
		var rf servarrRootFolder
		body, _ := io.ReadAll(r.Body)
		if err := json.Unmarshal(body, &rf); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		rf.ID = len(f.rootFolders) + 1
		f.rootFolders = append(f.rootFolders, rf)
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeRadarrServer) handleSettings(w http.ResponseWriter, r *http.Request) {
	if !f.requireKeyOK(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		var _ = json.NewEncoder(w).Encode(f.settings)
	case http.MethodPost:
		f.postSettingsCalls++
		body, _ := io.ReadAll(r.Body)
		var settings map[string]any
		if err := json.Unmarshal(body, &settings); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		f.settings = settings
		w.WriteHeader(http.StatusOK)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func TestConfigureRegistersDownloadClientAndRootFolder(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, srv.apiKey)
	cfg := &Config{
		APIKey: srv.apiKey,
		DownloadClient: DownloadClient{
			Name:     "qBittorrent",
			Host:     "qbittorrent",
			Port:     8080,
			Username: "admin",
			Password: "s3cr3t",
			Category: "movies",
		},
		RootFolder: "/data/media/movies",
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.addClientCalls != 1 {
		t.Errorf("expected 1 add download client call, got %d", srv.addClientCalls)
	}
	if srv.addFolderCalls != 1 {
		t.Errorf("expected 1 add root folder call, got %d", srv.addFolderCalls)
	}
	last := srv.downloadClients[len(srv.downloadClients)-1]
	if last.Category != "movies" || last.Hostname != "qbittorrent" {
		t.Errorf("stored download client = %+v, want category movies host qbittorrent", last)
	}
	lastFolder := srv.rootFolders[len(srv.rootFolders)-1]
	if lastFolder.Path != "/data/media/movies" {
		t.Errorf("stored root folder = %+v, want /data/media/movies", lastFolder)
	}
	if srv.lastApiKey != srv.apiKey {
		t.Errorf("X-Api-Key = %q, want %q", srv.lastApiKey, srv.apiKey)
	}
}

func TestConfigureIsIdempotent(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()

	srv.downloadClients = []servarrDLClient{{Name: "qBittorrent", Category: "movies"}}
	srv.rootFolders = []servarrRootFolder{{Path: "/data/media/movies"}}
	c := New(srv.URL, srv.apiKey)
	cfg := &Config{
		APIKey:         srv.apiKey,
		DownloadClient: DownloadClient{Name: "qBittorrent", Category: "movies"},
		RootFolder:     "/data/media/movies",
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.addClientCalls != 0 || srv.addFolderCalls != 0 {
		t.Errorf("expected no add calls on re-configure, got client=%d folder=%d",
			srv.addClientCalls, srv.addFolderCalls)
	}
}

func TestConfigureRejectsInvalidConfig(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, srv.apiKey)
	if err := c.Configure(context.Background(), "not-a-config"); err == nil {
		t.Fatal("expected error for non-*Config")
	}
}

func TestConfigureRejectsWrongAPIKey(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "wrong-key")
	cfg := &Config{APIKey: "wrong-key", DownloadClient: DownloadClient{Name: "qBittorrent"}}
	if err := c.Configure(context.Background(), cfg); err == nil {
		t.Fatal("expected error for wrong API key")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestVerifySuccessAndFailure(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	srv.downloadClients = []servarrDLClient{{Name: "qBittorrent", Category: "movies"}}
	srv.rootFolders = []servarrRootFolder{{Path: "/data/media/movies"}}

	c := New(srv.URL, srv.apiKey)
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	// Wrong category fails.
	srv.downloadClients = []servarrDLClient{{Name: "qBittorrent", Category: "tv"}}
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for wrong download client category")
	}

	// Missing root folder fails.
	srv.downloadClients = []servarrDLClient{{Name: "qBittorrent", Category: "movies"}}
	srv.rootFolders = []servarrRootFolder{{Path: "/data/media/tv"}}
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for missing /data/media/movies root folder")
	}
}

func TestVerifyWrongAPIKey(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "wrong-key")
	if err := c.Verify(context.Background()); err == nil {
		t.Fatal("expected error for wrong API key")
	}
}

func TestWaitHealthyLiveAndDead(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
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
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "")
	cfg := &Config{DownloadClient: DownloadClient{Name: "qBittorrent"}}
	if err := c.Configure(context.Background(), cfg); err == nil {
		t.Fatal("expected error when no API key against key-requiring server")
	}
}

func TestSyncSettingsUnverifiedWhenKeysAbsent(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	srv.settings = map[string]any{"someOtherSetting": true}
	c := New(srv.URL, srv.apiKey)
	// No hardlink key → no POST, no error (unverified path).
	if err := c.syncSettings(context.Background()); err != nil {
		t.Fatalf("syncSettings with no hardlink key: %v", err)
	}
	if srv.postSettingsCalls != 0 {
		t.Errorf("expected no settings POST when keys absent, got %d", srv.postSettingsCalls)
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("http://127.0.0.1:7878/", "key")
	if c.baseURL != "http://127.0.0.1:7878" {
		t.Errorf("baseURL = %q, expected trailing slash trimmed", c.baseURL)
	}
}

func TestVerifyUsesCheckWithinTimeout(t *testing.T) {
	srv := newFakeRadarrServer(t, true)
	defer srv.Close()
	srv.downloadClients = []servarrDLClient{{Name: "qBittorrent", Category: "movies"}}
	srv.rootFolders = []servarrRootFolder{{Path: "/data/media/movies"}}
	c := New(srv.URL, srv.apiKey)
	done := make(chan error, 1)
	go func() { done <- c.Verify(context.Background()) }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("Verify: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("Verify did not return")
	}
}
