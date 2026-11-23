//go:build e2e

package prowlarr

import (
	"context"
	"testing"
	"time"
)

// TestEWireApplications exercises WaitHealthy → Configure (register Sonarr and
// Radarr as Prowlarr applications) → Verify against a real
// linuxserver/prowlarr:3.0.2 container started from spikes/verify-compose
// (localhost:19696, key verify-prowlarr-...).
//
// Configure is idempotent: re-POSTing an application whose name is already
// present returns 400 "Should be unique", so Configure skips existing names
// and Verify must pass both before and after a second Configure.
//
// No indexers are added (repo rule: mock indexer only).
//
// Run: cd spikes/verify-compose && docker compose up -d prowlarr
//
//	go test ./internal/wiring/prowlarr -tags=e2e -run TestE2ERealProwlarr -v
func TestE2ERealProwlarr(t *testing.T) {
	const (
		baseURL = "http://127.0.0.1:19696"
		apiKey  = "verify-prowlarr-0000000000000001"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := New(baseURL, apiKey)
	if err := c.WaitHealthy(ctx); err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}

	cfg := &Config{
		APIKey: apiKey,
		Apps: []App{
			{Name: "Sonarr", Implementation: "Sonarr", ConfigContract: "SonarrSettings",
				BaseURL: "http://sonarr:8989", ProwlarrURL: "http://prowlarr:9696", APIKey: "verify-sonarr-0000000000000001"},
			{Name: "Radarr", Implementation: "Radarr", ConfigContract: "RadarrSettings",
				BaseURL: "http://radarr:7878", ProwlarrURL: "http://prowlarr:9696", APIKey: "verify-radarr-0000000000000001"},
		},
	}
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("Configure: %v", err)
	}
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify after configure: %v", err)
	}

	// Re-configure must be idempotent (duplicate names are skipped, not errors).
	if err := c.Configure(context.Background(), cfg); err != nil {
		t.Fatalf("re-Configure (idempotent): %v", err)
	}
	if err := c.Verify(context.Background()); err != nil {
		t.Fatalf("Verify after re-configure: %v", err)
	}
}
