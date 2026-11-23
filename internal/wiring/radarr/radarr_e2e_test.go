//go:build e2e

package radarr

import (
	"context"
	"testing"
	"time"
)

// TestEWireRootFolder exercises WaitHealthy → root-folder sync → verify against
// a real linuxserver/radarr:7.0.3 container started from spikes/verify-compose
// (localhost:17878, key verify-radarr-...).
//
// The download-client registration step of Configure is intentionally NOT
// covered here: the Servarr v3 POST /api/v3/downloadclient body requires a
// per-implementation config contract (implementation + configContract +
// fields) whose exact Qbittorrent shape could not be confirmed against the
// pinned container despite spec-following attempts (targetType null). It is
// recorded as not-verified in docs/build-log.md.
//
// Run: cd spikes/verify-compose && docker compose up -d radarr
//
//	docker exec vfy-radarr mkdir -p /data/media/movies && docker exec vfy-radarr chown abc:abc /data/media/movies
//	go test ./internal/wiring/radarr -tags=e2e -run TestE2ERealRadarr -v
func TestE2ERealRadarr(t *testing.T) {
	const (
		baseURL = "http://127.0.0.1:17878"
		apiKey  = "verify-radarr-0000000000000001"
		path    = "/data/media/movies"
	)
	ctx, cancel := context.WithTimeout(context.Background(), 60*time.Second)
	defer cancel()

	c := New(baseURL, apiKey)
	if err := c.WaitHealthy(ctx); err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}

	// Sync the media root folder (add if absent, skip if present).
	folders, err := c.Client.GetRootFolders(ctx)
	if err != nil {
		t.Fatalf("list root folders: %v", err)
	}
	present := false
	for _, rf := range folders {
		if rf.Path == path {
			present = true
			break
		}
	}
	if !present {
		if err := c.Client.AddRootFolder(ctx, path); err != nil {
			t.Fatalf("add root folder %q: %v", path, err)
		}
		folders, err = c.Client.GetRootFolders(ctx)
		if err != nil {
			t.Fatalf("list root folders after add: %v", err)
		}
	}

	found := false
	for _, rf := range folders {
		if rf.Path == path {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("root folder %q not present after sync: %+v", path, folders)
	}
}
