package servarr

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

type fakeServarrServer struct {
	*httptest.Server
	apiKey          string
	downloadClients []DownloadClient
	rootFolders     []RootFolder
	addClientCalls  int
	addFolderCalls  int
	requireKey      bool
}

func newFakeServarrServer(t *testing.T, requireKey bool) *fakeServarrServer {
	t.Helper()
	f := &fakeServarrServer{
		apiKey:          "test-key",
		downloadClients: []DownloadClient{},
		rootFolders:     []RootFolder{},
		requireKey:      requireKey,
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v3/downloadclient", f.handleDownloadClient)
	mux.HandleFunc("/api/v3/rootfolder", f.handleRootFolder)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeServarrServer) handleDownloadClient(w http.ResponseWriter, r *http.Request) {
	if f.requireKey && r.Header.Get("X-Api-Key") != f.apiKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.downloadClients)
	case http.MethodPost:
		f.addClientCalls++
		var dc DownloadClient
		if err := json.NewDecoder(r.Body).Decode(&dc); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		f.downloadClients = append(f.downloadClients, dc)
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func (f *fakeServarrServer) handleRootFolder(w http.ResponseWriter, r *http.Request) {
	if f.requireKey && r.Header.Get("X-Api-Key") != f.apiKey {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	switch r.Method {
	case http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(f.rootFolders)
	case http.MethodPost:
		f.addFolderCalls++
		var rf RootFolder
		if err := json.NewDecoder(r.Body).Decode(&rf); err != nil {
			http.Error(w, "bad json", http.StatusBadRequest)
			return
		}
		f.rootFolders = append(f.rootFolders, rf)
		w.WriteHeader(http.StatusCreated)
	default:
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
	}
}

func TestGetDownloadClients(t *testing.T) {
	srv := newFakeServarrServer(t, true)
	defer srv.Close()

	srv.downloadClients = []DownloadClient{{Name: "qBittorrent", Category: "tv"}}
	c := New(srv.URL, srv.apiKey)

	clients, err := c.GetDownloadClients(context.Background())
	if err != nil {
		t.Fatalf("GetDownloadClients: %v", err)
	}
	if len(clients) != 1 || clients[0].Name != "qBittorrent" {
		t.Errorf("clients = %+v, want one named qBittorrent", clients)
	}
}

func TestAddDownloadClient(t *testing.T) {
	srv := newFakeServarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, srv.apiKey)
	dc := DownloadClient{Name: "qBittorrent", Protocol: "torrent", Port: 8080, Category: "tv"}
	if err := c.AddDownloadClient(context.Background(), dc); err != nil {
		t.Fatalf("AddDownloadClient: %v", err)
	}
	if srv.addClientCalls != 1 {
		t.Errorf("expected 1 add download client call, got %d", srv.addClientCalls)
	}
	got := srv.downloadClients[len(srv.downloadClients)-1]
	if got.Category != "tv" || got.Port != 8080 {
		t.Errorf("stored client = %+v, want category tv port 8080", got)
	}
}

func TestAddDownloadClientRejectsWrongKey(t *testing.T) {
	srv := newFakeServarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "wrong-key")
	err := c.AddDownloadClient(context.Background(), DownloadClient{Name: "qBittorrent"})
	if err == nil {
		t.Fatal("expected error for wrong API key")
	} else if !strings.Contains(err.Error(), "401") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestGetRootFoldersAndAddRootFolder(t *testing.T) {
	srv := newFakeServarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, srv.apiKey)
	if err := c.AddRootFolder(context.Background(), "/data/media/tv"); err != nil {
		t.Fatalf("AddRootFolder: %v", err)
	}
	if srv.addFolderCalls != 1 {
		t.Errorf("expected 1 add root folder call, got %d", srv.addFolderCalls)
	}

	folders, err := c.GetRootFolders(context.Background())
	if err != nil {
		t.Fatalf("GetRootFolders: %v", err)
	}
	if len(folders) != 1 || folders[0].Path != "/data/media/tv" {
		t.Errorf("folders = %+v, want one at /data/media/tv", folders)
	}
}

func TestRequiresAPIKey(t *testing.T) {
	srv := newFakeServarrServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "")
	if _, err := c.GetDownloadClients(context.Background()); err == nil {
		t.Fatal("expected error when no API key configured against a key-requiring server")
	}
}

func TestNoKeyServerSucceedsWithoutHeader(t *testing.T) {
	// A server that does not require a key (e.g. auth not yet enabled).
	srv := newFakeServarrServer(t, false)
	defer srv.Close()

	c := New(srv.URL, "")
	if _, err := c.GetDownloadClients(context.Background()); err != nil {
		t.Fatalf("GetDownloadClients without key: %v", err)
	}
}

func TestReadBodyTruncates(t *testing.T) {
	long := strings.Repeat("x", 2000)
	resp := &http.Response{
		Status:     "200 OK",
		StatusCode: 200,
		Body:       io.NopCloser(strings.NewReader(long)),
	}
	if got := readBody(resp); len(got) != 1024 {
		t.Errorf("readBody len = %d, want 1024", len(got))
	}
}
