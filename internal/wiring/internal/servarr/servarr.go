// Package servarr is a shared low-level client for the Servarr v3 API
// (Sonarr, Radarr, Prowlarr). It handles authentication via the X-Api-Key
// header and minimal error wrapping; each app owns its own Configure/Verify
// logic under internal/wiring/<app> (PRD §12.1: one package per app).
//
// Authentication is a pre-generated API key injected at container creation via
// the app's APP__AUTH__APIKEY env var (verified for the pinned images in
// docs/spikes/01-servarr-api-keys.md — requests without/with a wrong key get
// 401, with the correct key they are authenticated).
//
// Servarr publishes no OpenAPI spec, so endpoint bodies below are the best
// current guess at the v3 resource shape and MUST be validated against the real
// pinned containers before trusting them. Nothing here is recorded as
// "impossible" without a real-container attempt (CLAUDE.md).
package servarr

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

// Client talks to a Servarr app's v3 API.
//
// All requests carry X-Api-Key and expect JSON. A 401 means the injected API
// key is wrong or missing — that is a configuration error, not a transient.
type Client struct {
	baseURL    string
	apiKey     string
	httpClient *http.Client
}

// New returns a Servarr v3 client for baseURL (e.g.
// http://127.0.0.1:8989) authenticated with apiKey. baseURL must not end with
// a slash.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// do performs an authenticated JSON request. A non-nil in is marshalled as the
// request body and given an application/json Content-Type.
func (c *Client) do(ctx context.Context, method, path string, in any) (*http.Response, error) {
	var body io.Reader
	if in != nil {
		b, err := json.Marshal(in)
		if err != nil {
			return nil, fmt.Errorf("servarr: marshal request: %w", err)
		}
		body = bytes.NewReader(b)
	}
	req, err := http.NewRequestWithContext(ctx, method, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	if in != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	resp, err := c.httpClient.Do(req)
	if err != nil {
		return nil, err
	}
	return resp, nil
}

// Get issues a GET to path.
func (c *Client) Get(ctx context.Context, path string) (*http.Response, error) {
	return c.do(ctx, http.MethodGet, path, nil)
}

// Post issues a POST to path with in as the JSON body.
func (c *Client) Post(ctx context.Context, path string, in any) (*http.Response, error) {
	return c.do(ctx, http.MethodPost, path, in)
}

// Put issues a PUT to path with in as the JSON body.
func (c *Client) Put(ctx context.Context, path string, in any) (*http.Response, error) {
	return c.do(ctx, http.MethodPut, path, in)
}

// readBody drains and returns the response body, capped at 1 KiB for error
// reporting. Caller must close resp.Body.
func readBody(resp *http.Response) string {
	if resp == nil || resp.Body == nil {
		return "<no body>"
	}
	defer func() { _ = resp.Body.Close() }()
	b, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
	return strings.TrimSpace(string(b))
}

// statusOK reports whether status is a 2xx Servarr success code.
func statusOK(status int) bool {
	return status >= 200 && status < 300
}

// DownloadClient is the subset of the v3 download client resource we read back
// to verify configuration. Servarr returns more fields than these; only those
// needed for wiring are decoded.
//
// POST /api/v3/downloadclient accepts a download client resource whose exact
// field set for qBittorrent is not documented by an OpenAPI spec — the body
// below (name/protocol/priority/enable/hostname/port/username/password/category)
// is the best-guess shape and must be confirmed against real pinned containers.
type DownloadClient struct {
	ID       int    `json:"id"`
	Name     string `json:"name"`
	Protocol string `json:"protocol"`
	Priority int    `json:"priority"`
	Enable   bool   `json:"enable"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	Username string `json:"username"`
	Password string `json:"password"`
	Category string `json:"category"`
	UpperDir string `json:"upperDirectory"`
}

// RootFolder is the subset of the v3 root folder resource we read back to
// verify the media root is registered.
type RootFolder struct {
	ID   int    `json:"id"`
	Path string `json:"path"`
}

// GetDownloadClients returns the configured download clients (GET
// /api/v3/downloadclient).
func (c *Client) GetDownloadClients(ctx context.Context) ([]DownloadClient, error) {
	resp, err := c.Get(ctx, "/api/v3/downloadclient")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if !statusOK(resp.StatusCode) {
		return nil, fmt.Errorf("servarr: GET /api/v3/downloadclient returned %s: %s", resp.Status, readBody(resp))
	}
	var clients []DownloadClient
	if err := json.NewDecoder(resp.Body).Decode(&clients); err != nil {
		return nil, fmt.Errorf("servarr: decode download clients: %w", err)
	}
	return clients, nil
}

// AddDownloadClient POSTs a download client resource (POST
// /api/v3/downloadclient).
func (c *Client) AddDownloadClient(ctx context.Context, dc DownloadClient) error {
	resp, err := c.Post(ctx, "/api/v3/downloadclient", dc)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if !statusOK(resp.StatusCode) {
		return fmt.Errorf("servarr: POST /api/v3/downloadclient returned %s: %s", resp.Status, readBody(resp))
	}
	return nil
}

// GetRootFolders returns the configured root folders (GET /api/v3/rootfolder).
func (c *Client) GetRootFolders(ctx context.Context) ([]RootFolder, error) {
	resp, err := c.Get(ctx, "/api/v3/rootfolder")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if !statusOK(resp.StatusCode) {
		return nil, fmt.Errorf("servarr: GET /api/v3/rootfolder returned %s: %s", resp.Status, readBody(resp))
	}
	var folders []RootFolder
	if err := json.NewDecoder(resp.Body).Decode(&folders); err != nil {
		return nil, fmt.Errorf("servarr: decode root folders: %w", err)
	}
	return folders, nil
}

// AddRootFolder POSTs a root folder resource (POST
// /api/v3/rootfolder) with just the path set.
func (c *Client) AddRootFolder(ctx context.Context, path string) error {
	resp, err := c.Post(ctx, "/api/v3/rootfolder", RootFolder{Path: path})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if !statusOK(resp.StatusCode) {
		return fmt.Errorf("servarr: POST /api/v3/rootfolder returned %s: %s", resp.Status, readBody(resp))
	}
	return nil
}

// GetSettings returns the raw v3 settings object (GET /api/v3/settings). The
// caller decodes only the keys it cares about; Servarr's settings schema has no
// published OpenAPI spec, so hardlink/naming keys are probed against real
// containers at wiring time and reported as unverified when absent.
func (c *Client) GetSettings(ctx context.Context) (map[string]any, error) {
	resp, err := c.Get(ctx, "/api/v3/settings")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if !statusOK(resp.StatusCode) {
		return nil, fmt.Errorf("servarr: GET /api/v3/settings returned %s: %s", resp.Status, readBody(resp))
	}
	var settings map[string]any
	if err := json.NewDecoder(resp.Body).Decode(&settings); err != nil {
		return nil, fmt.Errorf("servarr: decode settings: %w", err)
	}
	return settings, nil
}
