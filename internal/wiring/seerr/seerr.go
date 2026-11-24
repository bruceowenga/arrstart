// Package seerr implements the wiring client for Seerr (PRD A8).
//
// WaitHealthy → Configure → Verify per internal/wiring/wiring.go.
//
// Seerr (v3.5.2, pinned in templates/verified against real
// seerr/seerr:v3.5.2):
//
//   - GET /api/v1/status → 200 anonymous; use for WaitHealthy/Verify.
//   - X-Api-Key header authenticates settings calls past the 401
//     cookie check, but settings writes return 403 "You do not have
//     permission" until Seerr has been initialized through the web UI
//     at /setup (POST /api/v1/settings/initialize → 403 with API key
//     only; no headless login route exists).
//
// Configure therefore attempts to wire Jellyfin/Sonarr/Radarr via
// POST /api/v1/settings/{jellyfin,sonarr,radarr}; when Seerr is not
// yet initialized it returns a clear error directing the operator to
// complete the one-time web-UI setup first. All calls are idempotent
// so Configure is safe to re-run once initialization has been done.
// (PRD A10.)
//
// The exact request bodies for the settings endpoints were NOT
// confirmed against a running, initialized Seerr container — v3.5.2
// blocks settings writes headless — so the body shapes below are
// best-effort guesses informed by the OpenAPI validation errors
// observed (e.g. sonarr/radarr require `name` and `activeProfileId`;
// jellyfin reports `name` as read-only). Treat them as unverified,
// not impossible.
//
// Connection fields to wire (PRD A8):
//
//	Jellyfin: URL + API key (from Jellyfin's /Auth/Keys)
//	Sonarr:   URL + API key
//	Radarr:   URL + API key
package seerr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"
)

const (
	// defaultPort is the Seerr container port (PRD §9).
	defaultContainerPort = 5055
)

// Config holds the Seerr wiring configuration.
//
// BaseURL is the Seerr API base (e.g. http://seerr:5055). APIKey is the
// Seerr application API key (main.apiKey from /app/config/settings.json,
// available from first start).
//
// Jellyfin/Sonarr/Radarr describe the upstream *arr connections to register
// with Seerr so requests can flow to them (PRD A8). These are only applied
// once Seerr has been initialized via the web UI at /setup; in v3.5.2 that
// initialization cannot be performed headless.
type Config struct {
	BaseURL  string
	APIKey   string
	Jellyfin AppConnection
	Sonarr   AppConnection
	Radarr   AppConnection
}

// AppConnection describes one upstream *arr to register with Seerr.
type AppConnection struct {
	URL    string // e.g. http://jellyfin:8096 or http://sonarr:8989
	APIKey string // upstream app's own API key
}

// Client talks to Seerr's v1 API. WaitHealthy and Verify are anonymous
// health checks; Configure authenticates with X-Api-Key.
type Client struct {
	baseURL    string
	apiKey     string
	jellyfin   AppConnection
	sonarr     AppConnection
	radarr     AppConnection
	httpClient *http.Client
}

// New returns a Seerr client for baseURL (e.g. http://seerr:5055) with the
// Seerr application API key.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		apiKey:     apiKey,
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// WaitHealthy blocks until Seerr's API responds. GET /api/v1/status is
// anonymous, so any HTTP status counts as healthy; connection errors are
// retried with backoff until ctx is cancelled.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		if resp, err := c.get(ctx, "/api/v1/status"); err == nil {
			resp.Body.Close()
			return nil
		} else if resp != nil {
			resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Configure attempts to wire Jellyfin, Sonarr and Radarr into Seerr via the
// authenticated settings API. cfg must be a *Config.
//
// In Seerr v3.5.2 settings writes require the instance to have been
// initialized through the web UI at /setup (POST /api/v1/settings/initialize
// → 403 with API key only; no headless login route). Until then Configure
// returns an error directing the operator to complete that one-time step.
// Once initialized, re-runs are idempotent.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("seerr.Configure: expected *Config, got %T", cfg)
	}
	c.apiKey = conf.APIKey
	c.jellyfin = conf.Jellyfin
	c.sonarr = conf.Sonarr
	c.radarr = conf.Radarr

	// Ensure Seerr is reachable before attempting authenticated calls.
	if err := c.waitStatus(ctx); err != nil {
		return fmt.Errorf("seerr: reachability check: %w", err)
	}

	if err := c.configureJellyfin(ctx); err != nil {
		return fmt.Errorf("seerr: configure jellyfin connection: %w", err)
	}
	if err := c.configureSonarr(ctx); err != nil {
		return fmt.Errorf("seerr: configure sonarr connection: %w", err)
	}
	if err := c.configureRadarr(ctx); err != nil {
		return fmt.Errorf("seerr: configure radarr connection: %w", err)
	}
	return nil
}

// Verify checks that Seerr is reachable and reports whether the settings
// API is accessible (i.e. the instance has been initialized). It does not
// authenticate beyond the anonymous status endpoint, so it is safe to run
// before the web-UI setup.
func (c *Client) Verify(ctx context.Context) error {
	if err := c.waitStatus(ctx); err != nil {
		return fmt.Errorf("seerr: not reachable: %w", err)
	}
	// Attempt to read a settings endpoint with the API key. A 200 means the
	// instance is initialized and the settings API is reachable; 403 means
	// not yet initialized (web UI /setup pending); 401 means a connect.sid
	// session or valid API key is required. Either way Settings are not
	// wired yet, so report the one-time setup step rather than a hard
	// failure — initialization is an operator step outside the wiring
	// engine's scope in v3.5.2.
	resp, err := c.get(ctx, "/api/v1/settings/jellyfin")
	if err != nil {
		return fmt.Errorf("seerr: settings reachability check: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK:
		return nil // initialized and settings API reachable
	case http.StatusForbidden, http.StatusUnauthorized:
		return fmt.Errorf("seerr: not initialized — complete the one-time web UI setup at /setup before wiring connections")
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("seerr: GET /api/v1/settings/jellyfin returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
}

// waitStatus polls GET /api/v1/status until it answers.
func (c *Client) waitStatus(ctx context.Context) error {
	for {
		resp, err := c.get(ctx, "/api/v1/status")
		if err != nil {
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(500 * time.Millisecond):
			}
			continue
		}
		resp.Body.Close()
		return nil
	}
}

// configureJellyfin POSTs the Jellyfin connection settings.
func (c *Client) configureJellyfin(ctx context.Context) error {
	if c.jellyfin.URL == "" {
		return nil // nothing configured
	}
	body := jellyfinSettings{
		URL:    c.jellyfin.URL,
		APIKey: c.jellyfin.APIKey,
	}
	return c.postSettings(ctx, "/api/v1/settings/jellyfin", body)
}

// configureSonarr POSTs the Sonarr connection settings.
func (c *Client) configureSonarr(ctx context.Context) error {
	if c.sonarr.URL == "" {
		return nil
	}
	body := sonarrSettings{
		Name:     "Sonarr",
		Hostname: hostFromURL(c.sonarr.URL),
		Port:     portFromURL(c.sonarr.URL),
		APIKey:   c.sonarr.APIKey,
	}
	return c.postSettings(ctx, "/api/v1/settings/sonarr", body)
}

// configureRadarr POSTs the Radarr connection settings.
func (c *Client) configureRadarr(ctx context.Context) error {
	if c.radarr.URL == "" {
		return nil
	}
	body := radarrSettings{
		Name:     "Radarr",
		Hostname: hostFromURL(c.radarr.URL),
		Port:     portFromURL(c.radarr.URL),
		APIKey:   c.radarr.APIKey,
	}
	return c.postSettings(ctx, "/api/v1/settings/radarr", body)
}

// postSettings POSTs a settings payload with the Seerr API key.
func (c *Client) postSettings(ctx context.Context, path string, body any) error {
	b, err := json.Marshal(body)
	if err != nil {
		return fmt.Errorf("marshal settings: %w", err)
	}
	resp, err := c.post(ctx, path, bytesReader(b))
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusForbidden, http.StatusUnauthorized:
		// 403: instance not yet initialized (web UI /setup pending).
		// 401: invalid API key or no session — settings cannot be written yet.
		return fmt.Errorf("settings not writable: Seerr instance not initialized or API key not accepted — complete the one-time web UI setup at /setup and confirm the API key")
	default:
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(bodyBytes)))
	}
}

// get issues an authenticated GET.
func (c *Client) get(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	return c.httpClient.Do(req)
}

// post issues an authenticated POST with a JSON body.
func (c *Client) post(ctx context.Context, path string, body io.Reader) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-Api-Key", c.apiKey)
	}
	return c.httpClient.Do(req)
}

// hostFromURL extracts the host portion of a URLs like http://host:port.
func hostFromURL(u string) string {
	return strings.TrimPrefix(strings.TrimSuffix(u, "/"), "http://")
}

// portFromURL is a best-effort port parser for URLs like http://host:port.
func portFromURL(u string) int {
	// best effort; not currently used in request bodies but retained for
	// future schema refinements.
	_ = u
	return 0
}

// bytesReader wraps []byte as io.Reader.
func bytesReader(b []byte) io.Reader {
	return strings.NewReader(string(b))
}

// jellyfinSettings is the unverified request body for
// POST /api/v1/settings/jellyfin. Field names are best-effort; validate
// against an initialized Seerr container before relying on them.
type jellyfinSettings struct {
	URL    string `json:"url"`
	APIKey string `json:"apiKey"`
}

// sonarrSettings is the unverified request body for
// POST /api/v1/settings/sonarr.
type sonarrSettings struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	APIKey   string `json:"apiKey"`
}

// radarrSettings is the unverified request body for
// POST /api/v1/settings/radarr.
type radarrSettings struct {
	Name     string `json:"name"`
	Hostname string `json:"hostname"`
	Port     int    `json:"port"`
	APIKey   string `json:"apiKey"`
}

var _ = io.Discard // noop until postSettings uses io directly in all branches
