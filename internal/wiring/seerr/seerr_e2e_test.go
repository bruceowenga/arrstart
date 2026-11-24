//go:build e2e

package seerr

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"strings"
	"testing"
	"time"
)

// TestE2eHeadlessWire runs the client against a real
// seerr/seerr:v3.5.2 container on localhost:18051. It requires Docker and
// is only executed with `make e2e` / `go test -tags e2e`.
// It skips (never fails) when Docker is unavailable or ARRSTART_E2E != "1",
// so it never blocks CI in containerless environments.
//
// The pinned image tag must match templates/versions.yaml.
//
// Scope: Seerr v3.5.2 cannot be initialized headless (POST
// /api/v1/settings/initialize needs a connect.sid session; no headless
// login route exists — verified against a real container). This test
// therefore exercises what is checkable headless: anonymous health, a
// Verify that reports the not-initialized state, and a Configure that
// surfaces a clear /setup hint instead of swallowing the 403. A real
// operator completes the one-time web UI setup at /setup once, after
// which Configure's settings writes are expected to succeed.
const (
	e2eImage         = "seerr/seerr:v3.5.2"
	e2eContainerName = "arrstart-e2e-seerr"
	e2eHostPort      = "18051"
	e2eBaseURL       = "http://127.0.0.1:" + e2eHostPort
)

func TestE2eImagePinned(t *testing.T) {
	for _, name := range []string{"TestE2eHeadlessWire"} {
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

func TestE2eHeadlessWire(t *testing.T) {
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

	// Seerr v3.5.2 runs as uid/gid 1000 and needs /app/config writable by
	// that user (compose user: + chown per PRD G4). The e2e run here uses
	// --user root so the container starts in this sandbox without a pre-
	// chowned bind mount; operator deployments follow PRD G4.
	if _, err := docker(ctx,
		"run", "--rm", "--name", e2eContainerName,
		"-p", e2eHostPort+":5055",
		"-v", "arrstart-e2e-seerr-config:/app/config",
		"--user", "root",
		"--detach",
		e2eImage,
	); err != nil {
		t.Fatalf("docker run: %v", err)
	}

	if err := waitContainer(ctx, e2eContainerName); err != nil {
		t.Fatalf("container not running: %v", err)
	}

	c := New(e2eBaseURL, "")

	// WaitHealthy is anonymous — GET /api/v1/status answers 200 without an
	// API key and before the instance is initialized.
	if err := c.WaitHealthy(ctx); err != nil {
		t.Fatalf("WaitHealthy: %v", err)
	}

	// Verify with no API key: reachable, but settings not writable until
	// the one-time web UI setup at /setup is done.
	if err := c.Verify(ctx); err == nil {
		t.Fatal("Verify: expected not-initialized error before /setup")
	} else if !strings.Contains(err.Error(), "/setup") {
		t.Fatalf("Verify: expected /setup hint, got %v", err)
	}

	// Configure without initialization returns a clear operator-directed
	// error rather than silently swallowing the 403 from settings writes.
	conf := &Config{
		APIKey:   "not-a-real-key",
		Jellyfin: AppConnection{URL: "http://jellyfin:8096", APIKey: "jf-key"},
	}
	if err := c.Configure(ctx, conf); err == nil {
		t.Fatal("Configure: expected error for uninitialized instance")
	} else if !strings.Contains(err.Error(), "/setup") {
		t.Fatalf("Configure: expected /setup hint, got %v", err)
	}

	t.Logf("seerr e2e OK: health + /setup gate verified against %s", e2eImage)
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
