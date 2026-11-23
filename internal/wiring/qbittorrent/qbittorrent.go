// Package qbittorrent implements the wiring client for qBittorrent (PRD A6).
//
// Approach (Option A, verified against linuxserver/qbittorrent:5.3.0_v2.1.2-ls488
// in docs/spikes/02-qbittorrent-credentials.md):
//
//  1. Before the container first starts, SeedConfig writes qBittorrent.conf
//     with a PBKDF2 hash of a known admin password into [WebUI]. The container
//     then starts with no temporary password and accepts API login with the
//     known credential (verified: POST /api/v2/auth/login → 204).
//  2. After the container is healthy, Configure logs in and sets the default
//     save path and categories (tv, movies) via POST /api/v2/app/setPreferences.
//  3. Verify re-authenticates and checks app/version and app/preferences.
//
// The web UI password is stored under [WebUI] as:
//
//	Password_PBKDF2="@ByteArray(<base64(salt)>:<base64(derivedKey)>)"
//
// using PBKDF2-HMAC-SHA512, 100k iterations, 16-byte salt, 64-byte derived key.
// Salt precedes the derived key (verified computationally against qBittorrent
// issue #11680's documented adminadmin default hash).
package qbittorrent

import (
	"context"
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha512"
	"crypto/subtle"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/cookiejar"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// defaultIterations is the qBittorrent PBKDF2 iteration count (verified for
	// qBittorrent >= 4.2.0).
	defaultIterations = 100_000

	// defaultSaltLen is the salt length qBittorrent uses (verified: 16 bytes).
	defaultSaltLen = 16

	// defaultKeyLen is the derived key length qBittorrent uses (verified: 64
	// bytes = SHA-512 output).
	defaultKeyLen = 64
)

// Config holds the qBittorrent wiring configuration.
//
// SavePath is the default save path to configure (PRD A6: /data/downloads).
// Categories are the download categories to create (PRD A6: tv, movies).
// ConfigVolume is the host path of the qBittorrent config volume; SeedConfig
// writes qBittorrent.conf into <ConfigVolume>/qBittorrent/qBittorrent.conf,
// matching the container path /config/qBittorrent/qBittorrent.conf.
type Config struct {
	SavePath     string
	Categories   []string
	ConfigVolume string
}

// Client talks to the qBittorrent WebUI API. A cookie jar persists the
// QBT_SID session cookie across requests so authenticated endpoints work.
type Client struct {
	baseURL    string
	username   string
	password   string
	httpClient *http.Client
}

// New returns a qBittorrent client for baseURL (e.g.
// http://127.0.0.1:8080) with the given admin credentials. baseURL must not
// end with a slash.
func New(baseURL, username, password string) *Client {
	jar, _ := cookiejar.New(nil)
	return &Client{
		baseURL:  strings.TrimSuffix(baseURL, "/"),
		username: username,
		password: password,
		httpClient: &http.Client{
			Jar:     jar,
			Timeout: 10 * time.Second,
		},
	}
}

// WaitHealthy blocks until the qBittorrent WebUI responds to a request.
// An HTTP 401 counts as healthy: it means the server is up, just not yet
// authenticated. Connection errors are retried until ctx is cancelled.
//
// Verified against real containers: GET /api/v2/app/version responds once the
// WebUI is listening; the host-port login path works when host port equals
// container port with a properly formed Host header.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+"/api/v2/app/version", nil)
		if err != nil {
			return err
		}
		resp, err := c.httpClient.Do(req)
		if err == nil {
			_ = resp.Body.Close()
			// 200 means authenticated access (possible only with a session);
			// 401 means the server is up and needs auth. Either way it is
			// healthy enough to proceed; Configure performs the login.
			if resp.StatusCode == http.StatusOK || resp.StatusCode == http.StatusUnauthorized {
				return nil
			}
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// login authenticates and stores the QBT_SID session cookie in the client's
// cookie jar. Auth/login returns HTTP 204 with no response body (verified).
func (c *Client) login(ctx context.Context) error {
	form := url.Values{
		"username": {c.username},
		"password": {c.password},
	}
	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/v2/auth/login",
		strings.NewReader(form.Encode()),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/x-www-form-urlencoded")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("qBittorrent auth/login returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// Configure authenticates, then applies cfg: the default save path and
// categories are sent to POST /api/v2/app/setPreferences.
//
// cfg must be a *Config.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("qbittorrent.Configure: expected *Config, got %T", cfg)
	}
	if err := c.login(ctx); err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}

	body, err := json.Marshal(map[string]any{
		"save_path":  conf.SavePath,
		"categories": conf.Categories,
	})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodPost, c.baseURL+"/api/v2/app/setPreferences",
		strings.NewReader(string(body)),
	)
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusNoContent {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("qBittorrent app/setPreferences returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// Verify re-authenticates and checks that the app is reachable and that the
// configured save path and categories are present in preferences.
func (c *Client) Verify(ctx context.Context, expect Config) error {
	if err := c.login(ctx); err != nil {
		return fmt.Errorf("qbittorrent login: %w", err)
	}

	req, err := http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/api/v2/app/version", nil,
	)
	if err != nil {
		return err
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("qBittorrent app/version returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	_, _ = io.Copy(io.Discard, resp.Body)

	req, err = http.NewRequestWithContext(
		ctx, http.MethodGet, c.baseURL+"/api/v2/app/preferences", nil,
	)
	if err != nil {
		return err
	}
	resp, err = c.httpClient.Do(req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("qBittorrent app/preferences returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}

	var prefs struct {
		SavePath   string   `json:"save_path"`
		Categories []string `json:"categories"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&prefs); err != nil {
		return fmt.Errorf("qbittorrent: decode preferences: %w", err)
	}
	if prefs.SavePath != expect.SavePath {
		return fmt.Errorf("qbittorrent: save_path = %q, want %q", prefs.SavePath, expect.SavePath)
	}
	if !equalStringSlice(prefs.Categories, expect.Categories) {
		return fmt.Errorf("qbittorrent: categories = %v, want %v", prefs.Categories, expect.Categories)
	}
	return nil
}

// SeedConfig writes qBittorrent.conf with a pre-seeded PBKDF2 hash for the
// given admin password, so the container starts with a known credential and
// no temporary password is generated (verified: API login with the known
// password returns 204 on matching ports with default flags).
//
// It writes to <configVolume>/qBittorrent/qBittorrent.conf, matching the
// container path /config/qBittorrent/qBittorrent.conf (verified). The file
// is created with 0600 permissions because it contains a password hash.
// SeedConfig is idempotent: re-running overwrites the [WebUI] section in
// place.
func SeedConfig(configVolume, username, password string) error {
	if username != "admin" {
		return fmt.Errorf("qbittorrent: only the fixed admin user is supported, got %q", username)
	}
	if password == "" {
		return fmt.Errorf("qbittorrent: password must not be empty")
	}

	salt := make([]byte, defaultSaltLen)
	if _, err := io.ReadFull(rand.Reader, salt); err != nil {
		return fmt.Errorf("qbittorrent: generate salt: %w", err)
	}
	dk, err := pbkdf2.Key(sha512.New, password, salt, defaultIterations, defaultKeyLen)
	if err != nil {
		return fmt.Errorf("qbittorrent: derive key: %w", err)
	}
	hash := fmt.Sprintf(
		`@ByteArray(%s:%s)`,
		base64.StdEncoding.EncodeToString(salt),
		base64.StdEncoding.EncodeToString(dk),
	)

	confDir := filepath.Join(configVolume, "qBittorrent")
	if err := os.MkdirAll(confDir, 0o700); err != nil {
		return fmt.Errorf("qbittorrent: create config dir: %w", err)
	}
	confPath := filepath.Join(confDir, "qBittorrent.conf")
	content := fmt.Sprintf(
		"[WebUI]\nPassword_PBKDF2=\"%s\"\nHostHeaderValidation=true\n",
		hash,
	)
	if err := writeFileIfChanged(confPath, []byte(content), 0o600); err != nil {
		return fmt.Errorf("qbittorrent: write config: %w", err)
	}
	return nil
}

// VerifyPasswordHash recomputes the PBKDF2 hash for password using the salt
// extracted from an existing hash string and compares it in constant time.
// Useful for tests and for validating a pre-seeded hash without logging in.
func VerifyPasswordHash(hash, password string) (bool, error) {
	salt, dk, err := parseHash(hash)
	if err != nil {
		return false, err
	}
	expected, err := pbkdf2.Key(sha512.New, password, salt, defaultIterations, defaultKeyLen)
	if err != nil {
		return false, fmt.Errorf("qbittorrent: derive key: %w", err)
	}
	return subtle.ConstantTimeCompare(dk, expected) == 1, nil
}

// equalStringSlice reports whether a and b contain the same strings in the
// same order.
func equalStringSlice(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// writeFileIfChanged writes data to path with perm only when the existing
// file differs, avoiding unnecessary mtime updates on idempotent re-runs.
func writeFileIfChanged(path string, data []byte, perm os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytesEqual(existing, data) {
		return nil
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

// bytesEqual reports whether a and b are byte-for-byte identical.
func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// parseHash extracts the salt and derived key from a qBittorrent
// Password_PBKDF2 hash string of the form
//
//	@ByteArray(<base64(salt)>:<base64(derivedKey)>)
func parseHash(hash string) (salt, dk []byte, err error) {
	const prefix = "@ByteArray("
	const suffix = ")"
	if !strings.HasPrefix(hash, prefix) || !strings.HasSuffix(hash, suffix) {
		return nil, nil, fmt.Errorf("qbittorrent: invalid hash format %q", hash)
	}
	inner := hash[len(prefix) : len(hash)-len(suffix)]
	parts := strings.SplitN(inner, ":", 2)
	if len(parts) != 2 {
		return nil, nil, fmt.Errorf("qbittorrent: hash missing salt:key separator %q", hash)
	}
	salt, err = base64.StdEncoding.DecodeString(parts[0])
	if err != nil {
		return nil, nil, fmt.Errorf("qbittorrent: decode salt: %w", err)
	}
	dk, err = base64.StdEncoding.DecodeString(parts[1])
	if err != nil {
		return nil, nil, fmt.Errorf("qbittorrent: decode derived key: %w", err)
	}
	return salt, dk, nil
}
