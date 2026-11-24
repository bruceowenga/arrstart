// Package prowlarr implements the wiring client for Prowlarr (PRD A5).
//
// WaitHealthy → Configure → Verify per internal/wiring/wiring.go.
//
// Configure registers Sonarr and Radarr as Prowlarr applications so indexers
// sync automatically (POST /api/v1/applications), one application per
// configured app, skipping any application whose name is already present
// (re-POSTing a duplicate name returns 400 "Should be unique"). No indexers
// are ever added (repo rule: mock indexer only).
//
// Prowlarr serves the Servarr v1 API at /api/v1 (not /api/v3). The application
// request body below was confirmed against the real pinned container
// linuxserver/prowlarr:3.0.2: POST /api/v1/applications returned 201 and the
// app appeared on GET /api/v1/applications with id, implementationName and
// configContract echoed back.
//
// Authentication uses X-Api-Key injected via PROWLARR__AUTH__APIKEY — verified
// for the pinned image in docs/spikes/01-servarr-api-keys.md (401 without/wrong
// key, authenticated with correct key).
package prowlarr

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bruceowenga/arrstart/internal/wiring/internal/servarr"
)

// Config holds the Prowlarr wiring configuration.
//
// BaseURL is the Prowlarr v1 API base (e.g. http://prowlarr:9696). APIKey is
// the pre-generated Prowlarr admin key injected via PROWLARR__AUTH__APIKEY.
// Apps are the Sonarr/Radarr instances to register as Prowlarr applications so
// indexers sync automatically.
type Config struct {
	BaseURL string
	APIKey  string
	Apps    []App
}

// App describes a Sonarr/Radarr instance to register as a Prowlarr
// application. Implementation must be "Sonarr"/"Radarr" and ConfigContract
// "SonarrSettings"/"RadarrSettings" respectively (confirmed against the pinned
// container). BaseURL is how Prowlarr reaches the target *arr
// (e.g. http://sonarr:8989). APIKey is the target *arr's own API key, as
// shown in its Settings/General. ProwlarrURL is the Prowlarr URL the target
// *arr uses to reach Prowlarr (the target *arr validates it can connect there)
// — in a single compose network this matches BaseURL
// (http://prowlarr:9696); when the wiring client reaches Prowlarr on a
// different host-facing address, set ProwlarrURL to the internal compose URL.
// SyncCategories are the Newznab category IDs Prowlarr will sync for this
// application; omitted when empty.
type App struct {
	Name           string
	Implementation string
	ConfigContract string
	BaseURL        string
	APIKey         string
	ProwlarrURL    string
	SyncCategories []int
}

// Client talks to Prowlarr's v1 API via the shared servarr transport, carrying
// the X-Api-Key header on every request. Configure remembers the apps it was
// asked to register so Verify can re-check them (Verify receives no cfg).
type Client struct {
	baseURL string
	apiKey  string
	apps    []App
	*servarr.Client
}

// New returns a Prowlarr v1 client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		Client:  servarr.New(baseURL, apiKey),
	}
}

// WaitHealthy blocks until Prowlarr's v1 API responds. Any HTTP status counts
// as healthy (a 401 means the server is up but the API key is not yet
// accepted); connection errors are retried with backoff until ctx is
// cancelled.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		resp, err := c.Get(ctx, "/api/v1/system/status")
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

// applicationRequest is the JSON body for POST /api/v1/applications. Keys are
// lowercase per the confirmed Prowlarr application resource shape.
type applicationRequest struct {
	SyncLevel      string             `json:"syncLevel"`
	Name           string             `json:"name"`
	Implementation string             `json:"implementation"`
	ConfigContract string             `json:"configContract"`
	Fields         []applicationField `json:"fields"`
}

type applicationField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// existingApplication is the subset of the GET /api/v1/applications response
// we decode to detect an already-registered application by name.
type existingApplication struct {
	Name   string `json:"name"`
	ID     int    `json:"id"`
	Enable bool   `json:"enable"`
}

// Configure registers each configured app as a Prowlarr application
// (POST /api/v1/applications), skipping any whose name is already present.
// cfg must be a *Config. The requested apps are remembered so Verify can
// re-check them.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("prowlarr.Configure: expected *Config, got %T", cfg)
	}

	existing, err := c.listApplications(ctx)
	if err != nil {
		return fmt.Errorf("prowlarr: list applications: %w", err)
	}
	have := map[string]bool{}
	for _, a := range existing {
		have[a.Name] = true
	}

	for _, app := range conf.Apps {
		if have[app.Name] {
			continue
		}
		if err := c.addApplication(ctx, app); err != nil {
			return fmt.Errorf("prowlarr: add application %q: %w", app.Name, err)
		}
	}
	c.apps = conf.Apps
	return nil
}

// listApplications returns the registered Prowlarr applications (GET
// /api/v1/applications).
func (c *Client) listApplications(ctx context.Context) ([]existingApplication, error) {
	resp, err := c.Get(ctx, "/api/v1/applications")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("prowlarr: GET /api/v1/applications returned %s", resp.Status)
	}
	var apps []existingApplication
	if err := json.NewDecoder(resp.Body).Decode(&apps); err != nil {
		return nil, fmt.Errorf("prowlarr: decode applications: %w", err)
	}
	return apps, nil
}

// addApplication POSTs a single Prowlarr application (POST /api/v1/applications).
func (c *Client) addApplication(ctx context.Context, app App) error {
	prowlarrURL := app.ProwlarrURL
	if prowlarrURL == "" {
		prowlarrURL = c.baseURL
	}
	fields := []applicationField{
		{Name: "prowlarrUrl", Value: prowlarrURL},
		{Name: "baseUrl", Value: app.BaseURL},
		{Name: "apiKey", Value: app.APIKey},
	}
	if len(app.SyncCategories) > 0 {
		fields = append(fields, applicationField{Name: "syncCategories", Value: app.SyncCategories})
	}
	req := applicationRequest{
		SyncLevel:      "fullSync",
		Name:           app.Name,
		Implementation: app.Implementation,
		ConfigContract: app.ConfigContract,
		Fields:         fields,
	}
	resp, err := c.Post(ctx, "/api/v1/applications", req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("prowlarr: POST /api/v1/applications returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// indexerRequest is the JSON body for POST /api/v1/indexer. Field names match
// the Prowlarr indexer resource shape (implementation "Torznab",
// configContract "TorznabSettings"), confirmed by probing the pinned container.
// The url field is the indexer's Torznab API endpoint; categories are the
// Newznab category IDs Prowlarr syncs. No real indexer names or URLs are
// ever hardcoded (repo rule, ADR-0009).
type indexerRequest struct {
	Enable         bool           `json:"enable"`
	Implementation string         `json:"implementation"`
	ConfigContract string         `json:"configContract"`
	Name           string         `json:"name"`
	AppProfileId   int            `json:"appProfileId"`
	Priority       int            `json:"priority"`
	Fields         []indexerField `json:"fields"`
}

type indexerField struct {
	Name  string `json:"name"`
	Value any    `json:"value"`
}

// Indexer describes a Torznab indexer to add to Prowlarr. Only the fields
// needed to register the mock indexer for Gate 3 e2e are populated.
//
// Schema confirmed against real linuxserver/prowlarr:3.0.2: POST
// /api/v1/indexer requires top-level `enable`, `implementation`,
// `configContract`, `name`, `appProfileId` (>0) and `priority` (1-50), plus a
// `fields[]` entry named `baseUrl` holding the indexer's Torznab endpoint.
// Prowlarr attempts a live connectivity check to `baseUrl` at add time, so the
// URL must resolve from the Prowlarr container — in a single compose network
// that's the mock indexer's internal URL. No real indexer names or URLs are
// ever hardcoded (repo rule, ADR-0009).
type Indexer struct {
	Enable         bool
	Implementation string
	ConfigContract string
	Name           string
	AppProfileId   int
	Priority       int
	URL            string
	Categories     []int
}

// AddIndexer registers a Torznab indexer with Prowlarr (POST /api/v1/indexer).
// indexer.URL must be an address Prowlarr can reach (in a single compose
// network, the mock indexer's internal URL, e.g. http://mock-indexer:9999/api).
func (c *Client) AddIndexer(ctx context.Context, indexer Indexer) error {
	fields := []indexerField{
		{Name: "baseUrl", Value: indexer.URL},
	}
	if len(indexer.Categories) > 0 {
		fields = append(fields, indexerField{Name: "categories", Value: indexer.Categories})
	}
	req := indexerRequest{
		Enable:         indexer.Enable,
		Implementation: indexer.Implementation,
		ConfigContract: indexer.ConfigContract,
		Name:           indexer.Name,
		AppProfileId:   indexer.AppProfileId,
		Priority:       indexer.Priority,
		Fields:         fields,
	}
	resp, err := c.Post(ctx, "/api/v1/indexer", req)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusCreated {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("prowlarr: POST /api/v1/indexer returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// Verify re-checks that each app registered by Configure is still present as a
// Prowlarr application (matched by name).
func (c *Client) Verify(ctx context.Context) error {
	apps, err := c.listApplications(ctx)
	if err != nil {
		return err
	}
	have := map[string]bool{}
	for _, a := range apps {
		have[a.Name] = true
	}
	var missing []string
	for _, app := range c.apps {
		if !have[app.Name] {
			missing = append(missing, app.Name)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("prowlarr: application(s) not registered: %s", strings.Join(missing, ", "))
	}
	return nil
}
