package qbittorrent

import (
	"bytes"
	"context"
	"crypto/pbkdf2"
	"crypto/sha512"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"
)

// Hash verified computationally against qBittorrent issue #11680's documented
// adminadmin default hash. Salt precedes derived key.
const issue11680Hash = `@ByteArray(ARQ77eY1NUZaQsuDHbIMCA==:0WMRkYTUWVT9wVvdDtHAjU9b3b7uB8NR1Gur2hmQCvCDpm39Q+PsJRJPaCU51dEiz+dTzh8qbPsL8WkFljQYFQ==)`

func TestIssue11680AdminadminHashReproduced(t *testing.T) {
	salt, dk, err := parseHash(issue11680Hash)
	if err != nil {
		t.Fatalf("parseHash: %v", err)
	}
	computed, err := pbkdf2.Key(sha512.New, "adminadmin", salt, defaultIterations, defaultKeyLen)
	if err != nil {
		t.Fatalf("pbkdf2.Key: %v", err)
	}
	if !bytes.Equal(computed, dk) {
		t.Fatalf("PBKDF2 mismatch:\n got  %x\n want %x", computed, dk)
	}
	if len(salt) != defaultSaltLen {
		t.Fatalf("salt len = %d, want %d", len(salt), defaultSaltLen)
	}
	if len(dk) != defaultKeyLen {
		t.Fatalf("derived key len = %d, want %d", len(dk), defaultKeyLen)
	}
}

func TestVerifyPasswordHash(t *testing.T) {
	ok, err := VerifyPasswordHash(issue11680Hash, "adminadmin")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !ok {
		t.Fatal("expected adminadmin to match issue #11680 hash")
	}

	ok, err = VerifyPasswordHash(issue11680Hash, "wrong-password")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if ok {
		t.Fatal("expected wrong-password to NOT match")
	}

	_, err = VerifyPasswordHash("not-a-hash", "x")
	if err == nil {
		t.Fatal("expected error for invalid hash format")
	}
}

func TestParseHashRejectsMalformed(t *testing.T) {
	cases := []string{
		"",
		"no-prefix",
		"@ByteArray(nocolon)",
		"@ByteArray(bad!salt:key)",
		"@ByteArray(salt:bad!key)",
	}
	for _, c := range cases {
		if _, _, err := parseHash(c); err == nil {
			t.Errorf("parseHash(%q) expected error, got nil", c)
		}
	}
}

func TestSeedConfigWritesExpectedFormat(t *testing.T) {
	dir := t.TempDir()
	if err := SeedConfig(dir, "admin", "s3cr3t-P@ssw0rd"); err != nil {
		t.Fatalf("SeedConfig: %v", err)
	}

	confPath := filepath.Join(dir, "qBittorrent", "qBittorrent.conf")
	data, err := os.ReadFile(confPath)
	if err != nil {
		t.Fatalf("read config: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "[WebUI]") {
		t.Errorf("config missing [WebUI] section:\n%s", content)
	}
	if !strings.Contains(content, `Password_PBKDF2="@ByteArray(`) {
		t.Errorf("config missing Password_PBKDF2 line:\n%s", content)
	}
	if !strings.Contains(content, "HostHeaderValidation=true") {
		t.Errorf("config missing HostHeaderValidation=true:\n%s", content)
	}

	// Verify the embedded hash is valid and matches the password.
	hash := extractPasswordPBKDF2(t, content)
	ok, err := VerifyPasswordHash(hash, "s3cr3t-P@ssw0rd")
	if err != nil {
		t.Fatalf("VerifyPasswordHash: %v", err)
	}
	if !ok {
		t.Errorf("embedded hash does not match seeded password")
	}

	// Idempotency: re-running should not error and should produce an
	// equivalent hash (different salt is acceptable; both verify).
	if err := SeedConfig(dir, "admin", "s3cr3t-P@ssw0rd"); err != nil {
		t.Fatalf("re-SeedConfig: %v", err)
	}
	data2, _ := os.ReadFile(confPath)
	hash2 := extractPasswordPBKDF2(t, string(data2))
	ok2, err := VerifyPasswordHash(hash2, "s3cr3t-P@ssw0rd")
	if err != nil {
		t.Fatalf("VerifyPasswordHash after re-seed: %v", err)
	}
	if !ok2 {
		t.Errorf("re-seeded hash does not match seeded password")
	}
}

func TestSeedConfigRejectsInvalidInput(t *testing.T) {
	dir := t.TempDir()
	if err := SeedConfig(dir, "not-admin", "pw"); err == nil {
		t.Error("expected error for non-admin username")
	}
	if err := SeedConfig(dir, "admin", ""); err == nil {
		t.Error("expected error for empty password")
	}
}

func extractPasswordPBKDF2(t *testing.T, content string) string {
	t.Helper()
	const key = `Password_PBKDF2="`
	start := strings.Index(content, key)
	if start < 0 {
		t.Fatalf("Password_PBKDF2 not found in config:\n%s", content)
	}
	start += len(key)
	end := strings.Index(content[start:], `"`)
	if end < 0 {
		t.Fatalf("unterminated Password_PBKDF2 value:\n%s", content)
	}
	return content[start : start+end]
}

// ---- httptest server-backed client tests ----

type fakeQBServer struct {
	*httptest.Server
	preferences   map[string]any
	authRequired  bool
	lastAuthUser  string
	lastAuthPass  string
	setPrefsBody  string
	versionBody   string
	loginCalls    int
	setPrefsCalls int
	versionCalls  int
	prefsCalls    int
}

func newFakeQBServer(t *testing.T, authRequired bool) *fakeQBServer {
	t.Helper()
	f := &fakeQBServer{
		authRequired: authRequired,
		preferences:  map[string]any{},
	}
	mux := http.NewServeMux()
	mux.HandleFunc("/api/v2/auth/login", f.handleLogin)
	mux.HandleFunc("/api/v2/app/setPreferences", f.handleSetPreferences)
	mux.HandleFunc("/api/v2/app/version", f.handleVersion)
	mux.HandleFunc("/api/v2/app/preferences", f.handlePreferences)
	f.Server = httptest.NewServer(mux)
	t.Cleanup(f.Close)
	return f
}

func (f *fakeQBServer) handleLogin(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if err := r.ParseForm(); err != nil {
		http.Error(w, "bad form", http.StatusBadRequest)
		return
	}
	f.loginCalls++
	f.lastAuthUser = r.FormValue("username")
	f.lastAuthPass = r.FormValue("password")

	if f.authRequired &&
		(f.lastAuthUser != "admin" || f.lastAuthPass != "known-pw") {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	http.SetCookie(w, &http.Cookie{Name: "QBT_SID", Value: "test-session", Path: "/"})
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeQBServer) handleSetPreferences(w http.ResponseWriter, r *http.Request) {
	if r.Method != http.MethodPost {
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	if !f.hasSessionCookie(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	f.setPrefsCalls++
	body, _ := io.ReadAll(r.Body)
	f.setPrefsBody = string(body)

	var prefs map[string]any
	if err := json.Unmarshal(body, &prefs); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	f.preferences = prefs
	w.WriteHeader(http.StatusNoContent)
}

func (f *fakeQBServer) handleVersion(w http.ResponseWriter, r *http.Request) {
	f.versionCalls++
	if f.authRequired && !f.hasSessionCookie(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "text/plain")
	_, _ = fmt.Fprint(w, f.versionBody)
}

func (f *fakeQBServer) handlePreferences(w http.ResponseWriter, r *http.Request) {
	f.prefsCalls++
	if !f.hasSessionCookie(r) {
		http.Error(w, "unauthorized", http.StatusUnauthorized)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	if err := json.NewEncoder(w).Encode(f.preferences); err != nil {
		return
	}
}

func (f *fakeQBServer) hasSessionCookie(r *http.Request) bool {
	cookie, err := r.Cookie("QBT_SID")
	return err == nil && cookie.Value == "test-session"
}

func TestClientLoginSetsCookie(t *testing.T) {
	srv := newFakeQBServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "admin", "known-pw")
	if err := c.login(context.Background()); err != nil {
		t.Fatalf("login: %v", err)
	}
}

func TestClientConfigureAndVerify(t *testing.T) {
	srv := newFakeQBServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "admin", "known-pw")
	cfg := &Config{
		SavePath:   "/data/downloads",
		Categories: []string{"tv", "movies"},
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if srv.loginCalls != 1 {
		t.Errorf("expected 1 login during Configure, got %d", srv.loginCalls)
	}
	if srv.setPrefsCalls != 1 {
		t.Errorf("expected 1 setPreferences call, got %d", srv.setPrefsCalls)
	}
	if got := srv.preferences["save_path"]; got != "/data/downloads" {
		t.Errorf("save_path = %v, want /data/downloads", got)
	}
	if got := srv.preferences["categories"]; !reflect.DeepEqual(got, []any{"tv", "movies"}) {
		t.Errorf("categories = %v, want [tv movies]", got)
	}

	// Verify re-authenticates and checks preferences.
	srv.versionBody = "v5.3.0"
	srv.preferences = map[string]any{
		"save_path":  "/data/downloads",
		"categories": []string{"tv", "movies"},
	}
	if err := c.Verify(context.Background(), *cfg); err != nil {
		t.Fatalf("Verify: %v", err)
	}
	if srv.versionCalls != 1 {
		t.Errorf("expected 1 version call during Verify, got %d", srv.versionCalls)
	}
	if srv.prefsCalls != 1 {
		t.Errorf("expected 1 preferences call during Verify, got %d", srv.prefsCalls)
	}
}

func TestClientVerifyMismatchedSavePath(t *testing.T) {
	srv := newFakeQBServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "admin", "known-pw")
	srv.preferences = map[string]any{
		"save_path":  "/wrong/path",
		"categories": []string{"tv"},
	}
	cfg := Config{SavePath: "/data/downloads", Categories: []string{"tv", "movies"}}
	if err := c.Verify(context.Background(), cfg); err == nil {
		t.Fatal("expected error for mismatched save_path")
	} else if !strings.Contains(err.Error(), "/wrong/path") {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestClientWaitHealthy(t *testing.T) {
	// Server that is immediately up.
	srv := newFakeQBServer(t, true)
	defer srv.Close()
	c := New(srv.URL, "admin", "known-pw")
	if err := c.WaitHealthy(context.Background()); err != nil {
		t.Fatalf("WaitHealthy on live server: %v", err)
	}

	// Server that is never up (connection refused).
	down := New("http://127.0.0.1:1", "admin", "known-pw")
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()
	err := down.WaitHealthy(ctx)
	if err == nil {
		t.Fatal("expected error for unreachable server")
	}
}

func TestClientRejectsWrongCredentials(t *testing.T) {
	srv := newFakeQBServer(t, true)
	defer srv.Close()

	c := New(srv.URL, "admin", "wrong-pw")
	if err := c.login(context.Background()); err == nil {
		t.Fatal("expected login to fail with wrong password")
	}
}

func TestNewTrimsTrailingSlash(t *testing.T) {
	c := New("http://127.0.0.1:8080/", "admin", "pw")
	if c.baseURL != "http://127.0.0.1:8080" {
		t.Errorf("baseURL = %q, expected trailing slash trimmed", c.baseURL)
	}
}
