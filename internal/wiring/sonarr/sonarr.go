// Package sonarr implements the wiring client for Sonarr (PRD A3).
//
// WaitHealthy → Configure → Verify per internal/wiring/wiring.go.
//
// Configure does three things, each idempotent:
//  1. Registers qBittorrent as a download client (POST /api/v3/downloadclient)
//     when no client with the configured name exists.
//  2. Registers the media root folder /data/media/tv (POST
//     /api/v3/rootfolder) when no root folder with that path exists.
//  3. Probes /api/v3/settings for hardlink/naming keys so downloaded TV can be
//     imported by hardlink instead of copy. Servarr publishes no OpenAPI spec
//     for v3, so the settings keys below are unverified guesses probed against
//     real pinned containers; absent keys are reported as unverified, not
//     recorded as impossible (CLAUDE.md).
//
// Authentication uses X-Api-Key with the pre-generated API key injected at
// container creation via the SONARR__AUTH__APIKEY compose env var — verified
// for linuxserver/sonarr:4.0.20.3014-ls6 in docs/spikes/01-servarr-api-keys.md
// (401 without/wrong key, authenticated with correct key).
package sonarr

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/bruceowenga/arrstart/internal/wiring/internal/servarr"
)

// Config holds the Sonarr wiring configuration.
//
// BaseURL is the Sonarr v3 API base (e.g. http://sonarr:8989). APIKey is the
// pre-generated admin key injected via SONARR__AUTH__APIKEY. DownloadClient
// describes the qBittorrent instance Sonarr should send grabs to. RootFolder
// is the media root Sonarr manages (PRD A3: /data/media/tv).
type Config struct {
	BaseURL        string
	APIKey         string
	DownloadClient DownloadClient
	RootFolder     string
}

// DownloadClient describes a qBittorrent download client to register with
// Sonarr. Name must match what Sonarr expects (best-guess "qBittorrent" —
// confirm against the real pinned container).
type DownloadClient struct {
	Name     string
	Host     string
	Port     int
	Username string
	Password string
	Category string // "tv" for Sonarr
}

// Client talks to Sonarr's v3 API via the shared servarr transport, carrying
// the X-Api-Key header on every request.
type Client struct {
	baseURL string
	apiKey  string
	*servarr.Client
}

// New returns a Sonarr client.
func New(baseURL, apiKey string) *Client {
	return &Client{
		baseURL: strings.TrimSuffix(baseURL, "/"),
		apiKey:  apiKey,
		Client:  servarr.New(baseURL, apiKey),
	}
}

// WaitHealthy blocks until Sonarr's v3 API responds. Any HTTP status counts
// as healthy (a 401 means the server is up but the API key is not yet
// accepted); connection errors are retried with backoff until ctx is
// cancelled.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		resp, err := c.Client.Get(ctx, "/api/v3/system/status")
		if err == nil {
			resp.Body.Close()
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Configure registers qBittorrent as Sonarr's download client, adds the media
// root folder, and probes hardlink/naming settings. cfg must be a *Config.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("sonarr.Configure: expected *Config, got %T", cfg)
	}

	// 1. Sync the qBittorrent download client (add if absent by name).
	clients, err := c.Client.GetDownloadClients(ctx)
	if err != nil {
		return fmt.Errorf("sonarr: list download clients: %w", err)
	}
	present := false
	for _, dc := range clients {
		if dc.Name == conf.DownloadClient.Name && dc.Category == conf.DownloadClient.Category {
			present = true
			break
		}
	}
	if !present {
		if err := c.Client.AddDownloadClient(ctx, servarr.DownloadClient{
			Name:     conf.DownloadClient.Name,
			Protocol: "torrent",
			Priority: 1,
			Enable:   true,
			Hostname: conf.DownloadClient.Host,
			Port:     conf.DownloadClient.Port,
			Username: conf.DownloadClient.Username,
			Password: conf.DownloadClient.Password,
			Category: conf.DownloadClient.Category,
		}); err != nil {
			return fmt.Errorf("sonarr: add qBittorrent download client: %w", err)
		}
	}

	// 2. Sync the media root folder (add if absent by path).
	folders, err := c.Client.GetRootFolders(ctx)
	if err != nil {
		return fmt.Errorf("sonarr: list root folders: %w", err)
	}
	present = false
	for _, rf := range folders {
		if rf.Path == conf.RootFolder {
			present = true
			break
		}
	}
	if !present {
		if err := c.Client.AddRootFolder(ctx, conf.RootFolder); err != nil {
			return fmt.Errorf("sonarr: add root folder %q: %w", conf.RootFolder, err)
		}
	}

	// 3. Probe hardlink/naming settings (unverified — see package doc).
	if err := c.syncSettings(ctx); err != nil {
		return fmt.Errorf("sonarr: sync settings: %w", err)
	}
	return nil
}

// syncSettings reads /api/v3/settings and, when a hardlink/naming key is
// present, posts it back enabled. When no key is present it returns nil: the
// key names are best-guess and unconfirmed, and hardlink import is often a
// function of the download client and root folder sharing a filesystem rather
// than a toggle. Wiring results note this as unverified.
func (c *Client) syncSettings(ctx context.Context) error {
	settings, err := c.Client.GetSettings(ctx)
	if err != nil {
		return err
	}
	for _, key := range []string{"hardlink", "hardlinks", "useHardlinks"} {
		if _, ok := settings[key]; ok {
			settings[key] = true
			if err := c.postSettings(ctx, settings); err != nil {
				return fmt.Errorf("post settings %q (schema unverified): %w", key, err)
			}
			return nil
		}
	}
	return nil
}

// postSettings writes the full settings object back (POST /api/v3/settings).
func (c *Client) postSettings(ctx context.Context, settings map[string]any) error {
	resp, err := c.Client.Post(ctx, "/api/v3/settings", settings)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent && resp.StatusCode != http.StatusCreated {
		return fmt.Errorf("sonarr: POST /api/v3/settings returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	return nil
}

// Verify re-checks that qBittorrent is registered as the download client with
// the expected category and that the root folder path is present.
func (c *Client) Verify(ctx context.Context) error {
	clients, err := c.Client.GetDownloadClients(ctx)
	if err != nil {
		return fmt.Errorf("sonarr: list download clients: %w", err)
	}
	foundClient := false
	for _, dc := range clients {
		if dc.Name == "qBittorrent" && dc.Category == "tv" {
			foundClient = true
			break
		}
	}
	if !foundClient {
		return fmt.Errorf("sonarr: qBittorrent download client (category tv) not registered")
	}

	folders, err := c.Client.GetRootFolders(ctx)
	if err != nil {
		return fmt.Errorf("sonarr: list root folders: %w", err)
	}
	for _, rf := range folders {
		if rf.Path == "/data/media/tv" {
			return nil
		}
	}
	return fmt.Errorf("sonarr: root folder /data/media/tv not registered")
}
