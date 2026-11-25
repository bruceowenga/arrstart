// Package seerr implements the wiring client for Seerr (PRD A8).
//
// WaitHealthy → Configure → Verify per internal/wiring/wiring.go.
//
// WaitHealthy polls GET /api/v1/status (200 = up, no authentication
// required) — live verified against seerr/seerr:v3.5.2.
//
// Configure applies the connections Seerr should use for the wired apps
// (Jellyfin, Sonarr, Radarr — PRD A8) via
// POST /api/v1/settings/{jellyfin,sonarr,radarr}, authenticated with Seerr's
// main.apiKey in the X-Api-Key header (supported per Seerr's OpenAPI spec;
// main.apiKey is present from first start and read from
// <root>/config/seerr/settings.json by the bootstrap layer). Each payload omits
// readOnly fields (e.g. Jellyfin "name"); re-POSTing is idempotent
// (upsert semantics).
//
// Headless initialization gap — live verified against seerr/seerr:v3.5.2:
// Seerr v3.5.2 cannot create its admin user through the API —
// POST /api/v1/settings/initialize requires an authenticated connect.sid
// session (401 without one) and no /api/v1/auth/login route exists — so the
// admin user must be created once through the web UI at <BaseURL>/setup
// before Configure can write settings (settings writes return 403 while
// initialized=false). Configure returns an error wrapping errSeerrNotInitialized
// with guidance in that state; once initialized, Configure applies the
// connections and Verify re-checks them. This is a documented product gap
// (docs/adr/0006-seerr-image-user-config-ownership.md), not a client
// limitation: the wiring surface for PRD A8 is present and works once Seerr
// has been initialized.
//
// See docs/spikes/04-seerr-setup.md for the full real-container verification.
package seerr

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/bruceowenga/arrstart/internal/wiring/internal/servarr"
)

// errSeerrNotInitialized is the sentinel wrapped by Configure/Verify when
// Seerr has not yet been initialized — its admin user must be created once
// through the web UI at <BaseURL>/setup before settings (Jellyfin/Sonarr/
// Radarr connections) can be written. Callers use errors.Is to detect it.
var errSeerrNotInitialized = errors.New("seerr: not initialized")

// Config holds the Seerr wiring configuration.
//
// BaseURL is the Seerr API base (e.g. http://seerr:5055). APIKey is Seerr's
// main.apiKey, required for authenticated settings writes (X-Api-Key).
//
// Jellyfin/Sonarr/Radarr are the connections Seerr should make to the wired
// apps. A zero-value (empty URL) means that connection is not configured and
// is skipped by Configure/Verify.
type Config struct {
	BaseURL  string
	APIKey   string
	Jellyfin JellyfinConn
	Sonarr   SonarrConn
	Radarr   RadarrConn
}

// JellyfinConn describes the Seerr→Jellyfin connection.
type JellyfinConn struct {
	URL      string // Seerr-facing Jellyfin URL, e.g. http://jellyfin:8096
	Username string // Jellyfin admin username
	Password string // Jellyfin admin password
}

// SonarrConn describes the Seerr→Sonarr connection (PRD A8: default quality
// profile and root folder).
type SonarrConn struct {
	URL               string // e.g. http://sonarr:8989
	APIKey            string // Sonarr API key, from its Settings/General
	QualityProfileID  int    // activeProfileId — Seerr's default quality profile for Sonarr
	RootFolder        string // activeDirectory, e.g. /data/media/tv
	LanguageProfileID int    // activeLanguageProfileId
}

// RadarrConn describes the Seerr→Radarr connection (PRD A8: default quality
// profile and root folder).
type RadarrConn struct {
	URL                 string // e.g. http://radarr:7878
	APIKey              string // Radarr API key, from its Settings/General
	QualityProfileID    int    // activeProfileId — Seerr's default quality profile for Radarr
	RootFolder          string // activeDirectory, e.g. /data/media/movies
	MinimumAvailability string // e.g. "In Cinema"
}

// Client talks to Seerr's API. Configure authenticates with X-Api-Key and
// applies/appends the configured connections; the last configured Config is
// remembered so Verify can re-check it without being re-supplied.
type Client struct {
	baseURL    string
	apiKey     string
	configured *Config
	*servarr.Client
}

// New returns a Seerr client for baseURL (e.g. http://seerr:5055), authenticated
// with apiKey (Seerr's main.apiKey). baseURL must not end with a slash.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		Client:  servarr.New(baseURL, apiKey),
	}
}

// WaitHealthy blocks until Seerr's API responds. GET /api/v1/status answers
// 200 without authentication, so any HTTP status counts as healthy; connection
// errors are retried with backoff until ctx is cancelled.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		resp, err := c.Get(ctx, "/api/v1/status")
		if err == nil {
			_ = resp.Body.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Configure applies the configured Jellyfin/Sonarr/Radarr connections to
// Seerr (POST /api/v1/settings/{jellyfin,sonarr,radarr} + X-Api-Key). cfg
// must be a *Config. A connection with an empty URL is skipped.
//
// When Seerr is not yet initialized (no admin created via /setup), settings
// writes return 401/403 and Configure returns an error wrapping
// errSeerrNotInitialized with guidance to complete the one-time /setup step
// and re-run `arrstart rewire`. Once initialized, the connections are applied
// idempotently and remembered for Verify.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("seerr.Configure: expected *Config, got %T", cfg)
	}
	if conf.BaseURL == "" {
		return fmt.Errorf("seerr: Config.BaseURL is required")
	}
	c.baseURL = strings.TrimSuffix(conf.BaseURL, "/")
	c.apiKey = conf.APIKey
	c.Client = servarr.New(c.baseURL, c.apiKey)

	var errs []error
	if conf.Jellyfin.URL != "" {
		if err := c.configureJellyfin(ctx, conf.Jellyfin); err != nil {
			errs = append(errs, fmt.Errorf("seerr: jellyfin: %w", err))
		}
	}
	if conf.Sonarr.URL != "" {
		if err := c.configureSonarr(ctx, conf.Sonarr); err != nil {
			errs = append(errs, fmt.Errorf("seerr: sonarr: %w", err))
		}
	}
	if conf.Radarr.URL != "" {
		if err := c.configureRadarr(ctx, conf.Radarr); err != nil {
			errs = append(errs, fmt.Errorf("seerr: radarr: %w", err))
		}
	}
	if len(errs) > 0 {
		c.configured = nil
		return errors.Join(errs...)
	}
	c.configured = conf
	return nil
}

// configureJellyfin POSTs the Jellyfin connection settings. The "name" field
// is readOnly on the Seerr side and intentionally omitted.
func (c *Client) configureJellyfin(ctx context.Context, j JellyfinConn) error {
	resp, err := c.Post(ctx, "/api/v1/settings/jellyfin", jellyfinSettings{
		Hostname:  j.URL,
		AdminUser: j.Username,
		AdminPass: j.Password,
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return notInitializedErr(c.baseURL)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST /api/v1/settings/jellyfin returned %s: %s",
			resp.Status, strings.TrimSpace(string(body)))
	}
}

// configureSonarr POSTs the Sonarr connection settings.
func (c *Client) configureSonarr(ctx context.Context, s SonarrConn) error {
	resp, err := c.Post(ctx, "/api/v1/settings/sonarr", sonarrSettings{
		Name:                    "Sonarr",
		Hostname:                s.URL,
		Port:                    sonarrPort(s.URL),
		APIKey:                  s.APIKey,
		UseSsl:                  false,
		BaseUrl:                 "",
		ActiveProfileId:         s.QualityProfileID,
		ActiveProfileName:       "",
		ActiveDirectory:         s.RootFolder,
		ActiveLanguageProfileId: s.LanguageProfileID,
		Is4k:                    false,
		EnableSeasonFolders:     false,
		IsDefault:               true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return notInitializedErr(c.baseURL)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST /api/v1/settings/sonarr returned %s: %s",
			resp.Status, strings.TrimSpace(string(body)))
	}
}

// configureRadarr POSTs the Radarr connection settings.
func (c *Client) configureRadarr(ctx context.Context, r RadarrConn) error {
	resp, err := c.Post(ctx, "/api/v1/settings/radarr", radarrSettings{
		Name:                "Radarr",
		Hostname:            r.URL,
		Port:                radarrPort(r.URL),
		APIKey:              r.APIKey,
		UseSsl:              false,
		BaseUrl:             "",
		ActiveProfileId:     r.QualityProfileID,
		ActiveProfileName:   "",
		ActiveDirectory:     r.RootFolder,
		MinimumAvailability: r.MinimumAvailability,
		IsDefault:           true,
	})
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	switch resp.StatusCode {
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusUnauthorized, http.StatusForbidden:
		return notInitializedErr(c.baseURL)
	default:
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST /api/v1/settings/radarr returned %s: %s",
			resp.Status, strings.TrimSpace(string(body)))
	}
}

// Verify re-checks that every connection applied by Configure is still present
// in Seerr's settings (GET /api/v1/settings/{jellyfin,sonarr,radarr}).
func (c *Client) Verify(ctx context.Context) error {
	if c.configured == nil {
		return fmt.Errorf("seerr.Verify: client not configured")
	}
	if c.configured.Jellyfin.URL != "" {
		s, err := c.getJellyfinSettings(ctx)
		if err != nil {
			return err
		}
		if s.Hostname != c.configured.Jellyfin.URL {
			return fmt.Errorf("seerr: Jellyfin hostname = %q, want %q", s.Hostname, c.configured.Jellyfin.URL)
		}
	}
	if c.configured.Sonarr.URL != "" {
		s, err := c.getSonarrSettings(ctx)
		if err != nil {
			return err
		}
		if s.Hostname != c.configured.Sonarr.URL || s.ActiveProfileId != c.configured.Sonarr.QualityProfileID {
			return fmt.Errorf("seerr: Sonarr host/profile = %q/%d, want %q/%d",
				s.Hostname, s.ActiveProfileId, c.configured.Sonarr.URL, c.configured.Sonarr.QualityProfileID)
		}
	}
	if c.configured.Radarr.URL != "" {
		s, err := c.getRadarrSettings(ctx)
		if err != nil {
			return err
		}
		if s.Hostname != c.configured.Radarr.URL || s.ActiveProfileId != c.configured.Radarr.QualityProfileID {
			return fmt.Errorf("seerr: Radarr host/profile = %q/%d, want %q/%d",
				s.Hostname, s.ActiveProfileId, c.configured.Radarr.URL, c.configured.Radarr.QualityProfileID)
		}
	}
	return nil
}

// getJellyfinSettings returns Seerr's configured Jellyfin connection.
func (c *Client) getJellyfinSettings(ctx context.Context) (*jellyfinSettingsResponse, error) {
	resp, err := c.Get(ctx, "/api/v1/settings/jellyfin")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, notInitializedErr(c.baseURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/v1/settings/jellyfin returned %s", resp.Status)
	}
	var s jellyfinSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("seerr: decode Jellyfin settings: %w", err)
	}
	return &s, nil
}

// getSonarrSettings returns Seerr's configured Sonarr connection.
func (c *Client) getSonarrSettings(ctx context.Context) (*sonarrSettingsResponse, error) {
	resp, err := c.Get(ctx, "/api/v1/settings/sonarr")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, notInitializedErr(c.baseURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/v1/settings/sonarr returned %s", resp.Status)
	}
	var s sonarrSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("seerr: decode Sonarr settings: %w", err)
	}
	return &s, nil
}

// getRadarrSettings returns Seerr's configured Radarr connection.
func (c *Client) getRadarrSettings(ctx context.Context) (*radarrSettingsResponse, error) {
	resp, err := c.Get(ctx, "/api/v1/settings/radarr")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return nil, notInitializedErr(c.baseURL)
	}
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /api/v1/settings/radarr returned %s", resp.Status)
	}
	var s radarrSettingsResponse
	if err := json.NewDecoder(resp.Body).Decode(&s); err != nil {
		return nil, fmt.Errorf("seerr: decode Radarr settings: %w", err)
	}
	return &s, nil
}

// notInitializedErr wraps errSeerrNotInitialized with guidance to complete the
// one-time Seerr setup at <BaseURL>/setup and re-run `arrstart rewire`.
func notInitializedErr(baseURL string) error {
	return fmt.Errorf("%w: create the admin user once through the web UI at %s/setup, then re-run `arrstart rewire`",
		errSeerrNotInitialized, baseURL)
}

// sonarrPort extracts the port from a Sonarr URL, defaulting to 8989.
func sonarrPort(url string) int {
	return appPort(url, 8989)
}

// radarrPort extracts the port from a Radarr URL, defaulting to 7878.
func radarrPort(url string) int {
	return appPort(url, 7878)
}

// appPort parses the port from host:port, returning defaultPort when absent.
func appPort(url string, defaultPort int) int {
	url = strings.TrimPrefix(strings.TrimPrefix(url, "http://"), "https://")
	host := strings.SplitN(url, "/", 2)[0]
	parts := strings.Split(host, ":")
	if len(parts) == 2 {
		if port, err := strconv.Atoi(parts[1]); err == nil {
			return port
		}
	}
	return defaultPort
}

// ---- request/response DTOs ----

type jellyfinSettings struct {
	Hostname  string `json:"hostname"`
	AdminUser string `json:"adminUser"`
	AdminPass string `json:"adminPass"`
}

type sonarrSettings struct {
	Name                    string `json:"name"`
	Hostname                string `json:"hostname"`
	Port                    int    `json:"port"`
	APIKey                  string `json:"apiKey"`
	UseSsl                  bool   `json:"useSsl"`
	BaseUrl                 string `json:"baseUrl"`
	ActiveProfileId         int    `json:"activeProfileId"`
	ActiveProfileName       string `json:"activeProfileName"`
	ActiveDirectory         string `json:"activeDirectory"`
	ActiveLanguageProfileId int    `json:"activeLanguageProfileId"`
	Is4k                    bool   `json:"is4k"`
	EnableSeasonFolders     bool   `json:"enableSeasonFolders"`
	IsDefault               bool   `json:"isDefault"`
}

type radarrSettings struct {
	Name                string `json:"name"`
	Hostname            string `json:"hostname"`
	Port                int    `json:"port"`
	APIKey              string `json:"apiKey"`
	UseSsl              bool   `json:"useSsl"`
	BaseUrl             string `json:"baseUrl"`
	ActiveProfileId     int    `json:"activeProfileId"`
	ActiveProfileName   string `json:"activeProfileName"`
	ActiveDirectory     string `json:"activeDirectory"`
	MinimumAvailability string `json:"minimumAvailability"`
	IsDefault           bool   `json:"isDefault"`
}

type jellyfinSettingsResponse struct {
	Hostname  string `json:"hostname"`
	AdminUser string `json:"adminUser"`
}

type sonarrSettingsResponse struct {
	Hostname        string `json:"hostname"`
	ActiveProfileId int    `json:"activeProfileId"`
}

type radarrSettingsResponse struct {
	Hostname            string `json:"hostname"`
	ActiveProfileId     int    `json:"activeProfileId"`
	MinimumAvailability string `json:"minimumAvailability"`
}
