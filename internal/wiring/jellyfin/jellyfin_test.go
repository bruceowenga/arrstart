package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

// fakeServer simulates Jellyfin's startup/auth/library surface over httptest,
// mirroring jellyfin/jellyfin:12.4 as verified in the spike (see package doc):
//
//	GET /System/Info/Public              200 StartupWizardCompleted flag
//	POST /Startup/Configuration          204
//	GET  /Startup/User                   200 {"Name":"root"} placeholder until
//	                                     first user exists, then auth required
//	POST /Startup/User                   204 first-user creation
//	POST /Startup/Complete               204 flips StartupWizardCompleted true
//	POST /Users/AuthenticateByName       200 AccessToken with MediaBrowser header
//	GET/POST /Auth/Keys                  keys list / create (204)
//	GET/POST /Library/VirtualFolders     library list / add (204)
//
// Paths under /data/media/... are assumed present (a real container exposes
// them before Configure runs).
type fakeServer struct {
	wizardComplete bool
	userCreated    bool
	tokens         map[string]bool
	adminName      string
	adminPassword  string
	keys           []string // registered app names
	mediaHeaders   []string // every Authorization header seen (in order)
	libs           []libraryItem
}

func newFakeServer() *fakeServer {
	return &fakeServer{
		tokens: map[string]bool{},
		libs: []libraryItem{
			{Name: "Videos", CollectionType: "movies", LibraryOptions: libraryOptions{PathInfos: []mediaPathInfo{{Path: "/data/media/videos"}}}},
		},
	}
}

func (f *fakeServer) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if ah := r.Header.Get("Authorization"); ah != "" {
		f.mediaHeaders = append(f.mediaHeaders, ah)
	}
	p := r.URL.Path
	switch {
	case p == "/System/Info/Public" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"StartupWizardCompleted":%t}`, f.wizardComplete)
		return
	case p == "/Startup/Configuration" && r.Method == http.MethodPost:
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/Startup/User" && r.Method == http.MethodGet:
		if f.userCreated {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"Name":"root"}`)
		return
	case p == "/Startup/User" && r.Method == http.MethodPost:
		var u startupUser
		if err := json.NewDecoder(r.Body).Decode(&u); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if u.Name == "" || u.Password == "" {
			http.Error(w, "bad user", http.StatusBadRequest)
			return
		}
		f.adminName = u.Name
		f.adminPassword = u.Password
		f.userCreated = true
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/Startup/Complete" && r.Method == http.MethodPost:
		f.wizardComplete = true
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/Users/AuthenticateByName" && r.Method == http.MethodPost:
		var req authenticateByName
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if !strings.HasPrefix(r.Header.Get("Authorization"), `MediaBrowser`) {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		if req.Username != f.adminName || req.Pw != f.adminPassword {
			http.Error(w, "unauthorized", http.StatusUnauthorized)
			return
		}
		const token = "access-token"
		f.tokens[token] = true
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprintf(w, `{"AccessToken":"%s","IsAdministrator":true}`, token)
		return
	case p == "/Auth/Keys" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		items := make([]authKeyItem, 0, len(f.keys))
		for _, k := range f.keys {
			items = append(items, authKeyItem{AppName: k})
		}
		b, _ := json.Marshal(map[string]any{"Items": items})
		_, _ = w.Write(b)
		return
	case p == "/Auth/Keys" && r.Method == http.MethodPost:
		if app := r.URL.Query().Get("app"); app != "" {
			f.keys = append(f.keys, app)
		}
		w.WriteHeader(http.StatusNoContent)
		return
	case p == "/Library/VirtualFolders" && r.Method == http.MethodGet:
		w.Header().Set("Content-Type", "application/json")
		b, _ := json.Marshal(f.libs)
		_, _ = w.Write(b)
		return
	case p == "/Library/VirtualFolders" && r.Method == http.MethodPost:
		var lc libraryCreate
		if err := json.NewDecoder(r.Body).Decode(&lc); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		q := r.URL.Query()
		for _, pi := range lc.LibraryOptions.PathInfos {
			f.libs = append(f.libs, libraryItem{
				Name:           q.Get("name"),
				CollectionType: q.Get("collectionType"),
				LibraryOptions: libraryOptions{PathInfos: []mediaPathInfo{pi}},
			})
		}
		w.WriteHeader(http.StatusNoContent)
		return
	}
	http.NotFound(w, r)
}

// headerToken records every Authorization header issued with a Token so the
// caller can confirm authenticated calls carried the access token.
func (f *fakeServer) headerWithToken() bool {
	for _, h := range f.mediaHeaders {
		if strings.Contains(h, `Token=`) {
			return true
		}
	}
	return false
}

func TestWaitHealthy(t *testing.T) {
	srv := httptest.NewServer(newFakeServer())
	defer srv.Close()
	c := New(srv.URL)
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
	c := New(srv.URL)
	// Use a cancelled context so the retry loop exits immediately.
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := c.WaitHealthy(ctx); err != context.Canceled {
		t.Fatalf("WaitHealthy on down server with cancelled ctx: got %v, want context.Canceled", err)
	}
}

func TestConfigureFullFlow(t *testing.T) {
	f := newFakeServer()
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL)

	conf := &Config{AdminName: "admin", AdminPassword: "passw0rd"}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if c.token == "" {
		t.Fatal("Configure: no token stored")
	}
	if !f.wizardComplete {
		t.Fatal("Configure: wizard not completed")
	}
	if f.adminName != "admin" || f.adminPassword != "passw0rd" {
		t.Fatalf("Configure: wrong admin creds recorded: %q/%q", f.adminName, f.adminPassword)
	}
	if !sliceContains(f.keys, "arrstart") {
		t.Fatalf("Configure: wiring API key not created: %v", f.keys)
	}
	if len(f.mediaHeaders) == 0 {
		t.Fatal("Configure: no MediaBrowser header seen")
	}
	if !f.headerWithToken() {
		t.Fatalf("Configure: token missing from MediaBrowser headers: %v", f.mediaHeaders)
	}

	confirmed := map[string]libraryItem{}
	for _, l := range f.libs {
		if l.Name == "Videos" {
			continue // pre-existing fake library
		}
		for _, p := range libraryPaths(l) {
			confirmed[p] = l
		}
	}
	want := map[string]struct{ name, collectionType string }{
		"/data/media/tv":     {name: "TV Shows", collectionType: "tvshows"},
		"/data/media/movies": {name: "Movies", collectionType: "movies"},
	}
	for path, wantLib := range want {
		got, ok := confirmed[path]
		if !ok {
			t.Fatalf("Configure: library %q missing, got %v", path, f.libs)
		}
		if got.Name != wantLib.name || got.CollectionType != wantLib.collectionType {
			t.Fatalf("Configure: library %q = %+v, want %+v", path, got, wantLib)
		}
	}
}

func TestConfigureWithCustomLibraries(t *testing.T) {
	f := newFakeServer()
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL)

	conf := &Config{
		AdminName:     "admin",
		AdminPassword: "passw0rd",
		ServerName:    "mybox",
		AppName:       "wiring",
		Libraries:     []Library{{Name: "Anime", Path: "/data/media/anime", CollectionType: "tvshows"}},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if f.keys[0] != "wiring" {
		t.Fatalf("Configure: expected API key 'wiring', got %q", f.keys[0])
	}
	if !sliceContainsPath(f.libs, "/data/media/anime") {
		t.Fatalf("Configure: custom library missing, got %v", f.libs)
	}
}

func TestConfigureIdempotent(t *testing.T) {
	f := newFakeServer()
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL)

	conf := &Config{AdminName: "admin", AdminPassword: "passw0rd"}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure #1: %v", err)
	}
	keys1 := len(f.keys)
	libs1 := len(f.libs)
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure #2 (idempotent re-run): %v", err)
	}
	if len(f.keys) != keys1 {
		t.Fatalf("Configure re-run created duplicate API keys: %v", f.keys)
	}
	if len(f.libs) != libs1 {
		t.Fatalf("Configure re-run created duplicate libraries: %v", f.libs)
	}
	if f.adminName != "admin" {
		t.Fatalf("Configure re-run record changed (%q)", f.adminName)
	}
}

func TestVerifySuccess(t *testing.T) {
	f := newFakeServer()
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL)

	conf := &Config{AdminName: "admin", AdminPassword: "passw0rd"}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// A fresh client representing a later process, feeding only stored state.
	c2 := New(srv.URL)
	c2.adminName = conf.AdminName
	c2.adminPassword = conf.AdminPassword
	c2.appName = conf.AppName
	if err := c2.Verify(newCtx(t)); err != nil {
		t.Fatalf("Verify: %v", err)
	}
}

func TestVerifyMissingLibrary(t *testing.T) {
	f := newFakeServer()
	srv := httptest.NewServer(f)
	defer srv.Close()
	c := New(srv.URL)

	conf := &Config{
		AdminName:     "admin",
		AdminPassword: "passw0rd",
		Libraries:     []Library{{Name: "Extra", Path: "/data/media/extra", CollectionType: "movies"}},
	}
	if err := c.Configure(newCtx(t), conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	// Remove the extra library from the fake registry so Verify must fail.
	for i, l := range f.libs {
		found := false
		for _, p := range libraryPaths(l) {
			if p == "/data/media/extra" {
				found = true
				break
			}
		}
		if found {
			f.libs = append(f.libs[:i], f.libs[i+1:]...)
			break
		}
	}
	if err := c.Verify(newCtx(t)); err == nil {
		t.Fatal("Verify: expected missing-library error")
	} else if !strings.Contains(err.Error(), "/data/media/extra") {
		t.Fatalf("Verify: wrong error: %v", err)
	}
}

func TestVerifyBadCredentials(t *testing.T) {
	srv := httptest.NewServer(newFakeServer())
	defer srv.Close()
	c := New(srv.URL)
	c.adminName = "admin"
	c.adminPassword = "wrong"
	c.appName = "arrstart"
	if err := c.Verify(newCtx(t)); err == nil {
		t.Fatal("Verify: expected auth failure")
	}
}

// newCtx returns a context with a short timeout.
func newCtx(t *testing.T) context.Context {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	t.Cleanup(cancel)
	return ctx
}

func sliceContains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

// sliceContainsPath reports whether any registered library exposes the path.
func sliceContainsPath(libs []libraryItem, path string) bool {
	for _, l := range libs {
		for _, p := range libraryPaths(l) {
			if p == path {
				return true
			}
		}
	}
	return false
}
