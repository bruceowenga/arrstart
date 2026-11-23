//go:build e2e

package sonarr

import (
	"context"
	"testing"
	"time"
)

// TestEWireRootFolder exercises WaitHealthy → root-folder sync → verify against
// a real linuxserver/sonarr:4.0.20.3014-ls6 container started from
// spikes/verify-compose (localhost:18989, key verify-sonarr-...).
//
// The download-client registration step of Configure is intentionally NOT
// covered here: the Servarr v3 POST /api/v3/downloadclient body requires a
// per-implementation config contract (implementation + configContract +
// fields) whose exact Qbittorrent shape could not be confirmed against the
// pinned container despite spec-following attempts (targetType null). It is
// recorded as not-verified in docs/build-log.md.
//
// Run: cd spikes/verify-compose && docker compose up -d sonarr
//
//	docker exec vfy-sonarr mkdir -p /data/media/tv && docker exec vfy-sonarr chown abc:abc /data/media/tv
//	go test ./internal/wiring/sonarr -tags=e2e -run TestE2ERealSonarr -v
func TestE2ERealSonarr(t *testing.T) {
	const (
		baseURL = "http://127.0.0.1:18989"
		apiKey  = "verify-sonarr-0000000000000001"
		path    = "/data/media/tv"
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
