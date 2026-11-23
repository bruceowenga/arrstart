//go:build e2e

package jellyfin

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestE2eHeadlessWizard runs the full headless wire against a real
// jellyfin/jellyfin:12.4 container on localhost:18889. It requires Docker and
// is only executed with `make e2e` / `go test -tags e2e`.
// It skips (never fails) when Docker is unavailable or ARRSTART_E2E != "1", so
// it never blocks CI in containerless environments.
//
// The pinned image tag must match templates/versions.yaml.
const (
	e2eImage         = "jellyfin/jellyfin:12.4"
	e2eContainerName = "arrstart-e2e-jellyfin"
	e2eHostPort      = "18889"
	e2eBaseURL       = "http://127.0.0.1:" + e2eHostPort
	e2eAdminName     = "arrstart-admin"
	e2eAdminPassword = "arrstart-secret-changeme"
)

func TestE2eImagePinned(t *testing.T) {
	for _, name := range []string{"TestE2eHeadlessWizard"} {
		t.Run(name, func(t *testing.T) {
			versions, err := os.ReadFile("../../../../templates/versions.yaml")
			if err != nil {
				t.Skipf("cannot read versions.yaml: %v", err)
			}
			if !strings.Contains(string(versions), e2eImage) {
				t.Fatalf("e2e image %q not pinned in templates/versions.yaml", e2eImage)
			}
		})
	}
}

func TestE2eHeadlessWizard(t *testing.T) {
	if os.Getenv("ARRSTART_E2E") != "1" {
		t.Skip("set ARRSTART_E2E=1 to run real-container e2e tests")
	}
	if _, err := docker(context.Background(), "ps", "--format", "{{.Names}}"); err != nil {
		t.Skipf("docker unavailable: %v", err)
	}

	ctx, cancel := context.WithTimeout(context.Background(), 6*time.Minute)
	defer cancel()

	// Start from a clean slate; ignore failure if the container doesn't exist.
	_, _ = docker(ctx, "rm", "-f", e2eContainerName)

	// The media paths must exist on disk inside the container before Jellyfin
	// registers libraries against them (Jellyfin validates path existence).
	if _, err := docker(ctx,
		"run", "--rm", "--name", e2eContainerName,
		"-p", e2eHostPort+":8096",
		"-v", "arrstart-e2e-jf-media:/data/media",
		e2eImage,
		"-d", "media/library/path=/data/media/tv",
		"-d", "media/library/path=/data/media/movies",
	); err != nil {
		t.Fatalf("docker run: %v", err)
	}

	if err := waitContainer(ctx, e2eContainerName); err != nil {
		t.Fatalf("container not running: %v", err)
	}

	c := New(e2eBaseURL)
	conf := &Config{
		AdminName:     e2eAdminName,
		AdminPassword: e2eAdminPassword,
		ServerName:    "arrstart-e2e",
		AppName:       "arrstart",
		Libraries: []Library{
			{Name: "TV Shows", Path: "/data/media/tv", CollectionType: "tvshows"},
			{Name: "Movies", Path: "/data/media/movies", CollectionType: "movies"},
		},
	}

	if err := c.WaitHealthy(ctx); err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}
	if err := c.Configure(ctx, conf); err != nil {
		t.Fatalf("Configure: %v", err)
	}

	// Idempotent re-run must succeed without duplicates or errors.
	if err := c.Configure(ctx, conf); err != nil {
		t.Fatalf("Configure (repeat): %v", err)
	}

	// Verify re-authenticates and re-checks state with no remembered token.
	c2 := New(e2eBaseURL)
	c2.adminName = e2eAdminName
	c2.adminPassword = e2eAdminPassword
	c2.appName = "arrstart"
	if err := c2.Verify(ctx); err != nil {
		t.Fatalf("Verify: %v", err)
	}

	t.Logf("jellyfin e2e OK: wizard, auth, API key, libraries verified against %s", e2eImage)
}

func docker(ctx context.Context, args ...string) (string, error) {
	cmd := exec.CommandContext(ctx, "docker", args...)
	out, err := cmd.CombinedOutput()
	return string(out), err
}

func waitContainer(ctx context.Context, name string) error {
	deadline, _ := ctx.Deadline()
	for time.Now().Before(deadline) {
		if out, err := docker(context.Background(), "inspect", "--format", "{{.State.Status}}", name); err == nil && strings.TrimSpace(out) == "running" {
			return nil
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
	return fmt.Errorf("timed out waiting for container %s", name)
}
