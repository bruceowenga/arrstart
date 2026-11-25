package generator

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"sort"
	"strings"
	"testing"

	"github.com/bruceowenga/arrstart/templates"
)

// update, when set with -update, rewrites the golden fixture files instead of
// comparing against them. Run `go test ./internal/generator/ -run
// TestGoldenQbittorrentLinux -update` once to create them, then review by eye.
var update = updateFlag(false)

type updateFlag bool

func (f *updateFlag) String() string   { return fmt.Sprintf("%v", *f) }
func (f *updateFlag) Set(string) error { *f = true; return nil }
func (f *updateFlag) IsBoolFlag() bool { return true }

func init() {
	flag.Var(&update, "update", "rewrite golden fixtures instead of comparing against them")
}

// goldenCreds are obviously fake credential values.
var goldenCreds = map[string]string{
	"QBT_APIKEY": "FAKE-QBT-KEY-0001",
}

var baseOpts = Options{
	MediaRoot: "/srv/media",
	HostOS:    "linux",
	PUID:      1000,
	PGID:      1000,
	TZ:        "UTC",
}

var qbtSpec = []AppSpec{{Name: "qbittorrent"}}

func TestGoldenQbittorrentLinux(t *testing.T) {
	if !update {
		t.Skip("use -update to (re)create golden fixtures")
	}
	compose, env, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	dir := "testdata/qbittorrent-linux"
	if err := os.MkdirAll(dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "compose.yaml"), compose, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, ".env"), env, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote golden fixtures under %s", dir)
}

func requireGolden(t *testing.T) ([]byte, []byte) {
	compose, err := os.ReadFile("testdata/qbittorrent-linux/compose.yaml")
	if err != nil {
		t.Fatalf("read golden compose: %v (run with -update to create)", err)
	}
	env, err := os.ReadFile("testdata/qbittorrent-linux/.env")
	if err != nil {
		t.Fatalf("read golden env: %v (run with -update to create)", err)
	}
	return compose, env
}

func TestGoldenQbittorrentLinuxCompare(t *testing.T) {
	compose, env, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	wantCompose, wantEnv := requireGolden(t)
	if !bytes.Equal(compose, wantCompose) {
		t.Fatalf("compose mismatch\ngot:\n%s\n\nwant:\n%s", compose, wantCompose)
	}
	if !bytes.Equal(env, wantEnv) {
		t.Fatalf("env mismatch\ngot:\n%s\n\nwant:\n%s", env, wantEnv)
	}
}

func TestDeterministic(t *testing.T) {
	multi := []AppSpec{{Name: "qbittorrent"}, {Name: "seerr"}}
	reversed := []AppSpec{{Name: "seerr"}, {Name: "qbittorrent"}}
	a, _, err := Generate(multi, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	b, _, err := Generate(reversed, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("spec order changed output\na:\n%s\n\nb:\n%s", a, b)
	}
	// identical inputs must be identical as well
	c, _, err := Generate(multi, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !bytes.Equal(a, c) {
		t.Fatal("repeat Generate() with identical inputs differs")
	}
}

func TestWindowsPaths(t *testing.T) {
	winOpts := Options{
		MediaRoot: `C:\Media`,
		HostOS:    "windows",
		PUID:      0,
		PGID:      0,
		TZ:        "",
	}
	compose, env, err := Generate(qbtSpec, goldenCreds, winOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !strings.Contains(string(compose), "C:/Media/config/qbittorrent:/config") {
		t.Errorf("compose missing normalized windows config volume:\n%s", compose)
	}
	if !strings.Contains(string(compose), "C:/Media/data:/data") {
		t.Errorf("compose missing normalized windows data volume:\n%s", compose)
	}
	if strings.Contains(string(compose), " user:") {
		t.Errorf("compose must not set user: on windows:\n%s", compose)
	}
	if strings.Contains(string(compose), "UMASK") {
		t.Errorf("compose must not set UMASK on windows:\n%s", compose)
	}
	envStr := string(env)
	for _, key := range []string{"PUID=", "PGID=", "UMASK="} {
		if strings.Contains(envStr, key) {
			t.Errorf(".env must not contain %q on windows:\n%s", key, envStr)
		}
	}
}

func TestPortConflictQbittorrent(t *testing.T) {
	conflict := []AppSpec{{Name: "qbittorrent", HostPortOverride: map[int]int{8080: 18080}}}
	compose, env, err := Generate(conflict, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !strings.Contains(string(compose), " - \"18080:18080\"") {
		t.Errorf("expected equal host/container remap 18080:18080:\n%s", compose)
	}
	envStr := string(env)
	if !strings.Contains(envStr, "WEBUI_PORT=18080") {
		t.Errorf("expected WEBUI_PORT=18080 in .env:\n%s", envStr)
	}
	// default case has no WEBUI_PORT
	_, envDefault, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if strings.Contains(string(envDefault), "WEBUI_PORT") {
		t.Errorf("default render must not set WEBUI_PORT:\n%s", envDefault)
	}
}

func TestDigestRendering(t *testing.T) {
	// qbittorrent pin has no digest -> image:tag form, never @digest.
	compose, _, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	want := "image: linuxserver/qbittorrent:5.3.0_v2.1.2-ls488"
	if !strings.Contains(string(compose), want) {
		t.Errorf("expected %q in compose:\n%s", want, compose)
	}
	if strings.Contains(string(compose), "@sha256") {
		t.Errorf("no-digest pin must not render @digest:\n%s", compose)
	}
	// seerr pin carries a digest in versions.yaml (data model). Rendering it as
	// image@sha256 requires templates.Image() to append @digest (tracked).
	if d := templates.Pins["seerr"].Digest; d == "" {
		t.Fatal("seerr pin missing digest in versions.yaml")
	}
}

func TestNoLatestAndNoSecretsInCompose(t *testing.T) {
	compose, env, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if strings.Contains(string(compose), ":latest") {
		t.Fatalf("compose must not contain :latest:\n%s", compose)
	}
	// cred values live only in .env; compose references them only as ${VAR}.
	for _, v := range goldenCreds {
		if strings.Contains(string(compose), v) {
			t.Fatalf("compose contains credential value %q:\n%s", v, compose)
		}
	}
	// .env is sorted and has no duplicate keys.
	lines := strings.Split(string(env), "\n")
	seen := make(map[string]bool)
	var keys []string
	for _, line := range lines {
		if line == "" {
			continue
		}
		k, _, _ := strings.Cut(line, "=")
		keys = append(keys, k)
		if seen[k] {
			t.Fatalf(".env has duplicate key %q", k)
		}
		seen[k] = true
	}
	sorted := append([]string(nil), keys...)
	sort.Strings(sorted)
	for i := range keys {
		if keys[i] != sorted[i] {
			t.Fatalf(".env keys not sorted: %v", keys)
		}
	}
}

func TestRootRefused(t *testing.T) {
	rootOpts := Options{
		MediaRoot: "/srv/media",
		HostOS:    "linux",
		PUID:      0,
		PGID:      1000,
		TZ:        "UTC",
	}
	if _, _, err := Generate(qbtSpec, goldenCreds, rootOpts); err == nil {
		t.Fatal("expected error for PUID 0 on linux")
	} else if !strings.Contains(err.Error(), "refusing to run the stack as root") {
		t.Fatalf("unexpected error: %v", err)
	}
}

func TestComposeConfigValid(t *testing.T) {
	if _, err := exec.LookPath("docker"); err != nil {
		if os.Getenv("CI") == "true" {
			t.Fatal("docker required in CI but not available")
		}
		t.Skip("docker not available locally")
	}
	compose, env, err := Generate(qbtSpec, goldenCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	dir := t.TempDir()
	cf := filepath.Join(dir, "docker-compose.yml")
	ef := filepath.Join(dir, ".env")
	if err := os.WriteFile(cf, compose, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(ef, env, 0600); err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("docker", "compose", "-f", cf, "--env-file", ef, "config")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("docker compose config failed: %v\n%s", err, out)
	}
	t.Logf("docker compose config OK:\n%s", out)
}
