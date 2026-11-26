package generator

import (
	"bytes"
	"flag"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

var update = updateFlag(false)

type updateFlag bool

func (f *updateFlag) String() string   { return fmt.Sprintf("%v", *f) }
func (f *updateFlag) Set(string) error { *f = true; return nil }
func (f *updateFlag) IsBoolFlag() bool { return true }

func init() { flag.Var(&update, "update", "rewrite golden fixtures instead of comparing against them") }

// obviously fake credential values
var qbtCreds = map[string]string{"QBT_APIKEY": "FAKE-QBT-KEY-0001"}
var moviesCreds = map[string]string{
	"RADARR_APIKEY":   "FAKE-RADARR-KEY-0001",
	"PROWLARR_APIKEY": "FAKE-PROWLARR-KEY-0001",
	"QBT_APIKEY":      "FAKE-QBT-KEY-0001",
}
var familyCreds = map[string]string{
	"SONARR_APIKEY":   "FAKE-SONARR-KEY-0001",
	"RADARR_APIKEY":   "FAKE-RADARR-KEY-0001",
	"PROWLARR_APIKEY": "FAKE-PROWLARR-KEY-0001",
	"QBT_APIKEY":      "FAKE-QBT-KEY-0001",
}

var baseOpts = Options{MediaRoot: "/srv/media", HostOS: "linux", PUID: 1000, PGID: 1000, TZ: "UTC"}
var winOpts = Options{MediaRoot: `C:\Media`, HostOS: "windows", TZ: "UTC"}

var qbtSpec = []AppSpec{{Name: "qbittorrent"}}
var moviesSpec = []AppSpec{{Name: "radarr"}, {Name: "prowlarr"}, {Name: "qbittorrent"}, {Name: "jellyfin"}, {Name: "seerr"}}
var familySpec = []AppSpec{{Name: "sonarr"}, {Name: "radarr"}, {Name: "prowlarr"}, {Name: "qbittorrent"}, {Name: "jellyfin"}, {Name: "seerr"}}

type gold struct {
	name  string
	spec  []AppSpec
	creds map[string]string
	opts  Options
	dir   string
}

var goldens = []gold{
	{"qbittorrent-linux", qbtSpec, qbtCreds, baseOpts, "testdata/qbittorrent-linux"},
	{"movies-linux", moviesSpec, moviesCreds, baseOpts, "testdata/movies-linux"},
	{"family-linux", familySpec, familyCreds, baseOpts, "testdata/family-linux"},
	{"family-windows", familySpec, familyCreds, winOpts, "testdata/family-windows"},
}

func writeGolden(t *testing.T, g gold) {
	t.Helper()
	compose, env, err := Generate(g.spec, g.creds, g.opts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if err := os.MkdirAll(g.dir, 0755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.dir, "compose.yaml"), compose, 0644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(g.dir, ".env"), env, 0600); err != nil {
		t.Fatal(err)
	}
	t.Logf("wrote golden fixtures under %s", g.dir)
}

// TestGoldenFixtures: run with -update to (re)create all four fixture dirs.
func TestGoldenFixtures(t *testing.T) {
	if !update {
		t.Skip("use -update to (re)create golden fixtures")
	}
	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) { writeGolden(t, g) })
	}
}

func requireGolden(t *testing.T, dir string) ([]byte, []byte) {
	t.Helper()
	compose, err := os.ReadFile(filepath.Join(dir, "compose.yaml"))
	if err != nil {
		t.Fatalf("read golden compose: %v (run with -update to create)", err)
	}
	env, err := os.ReadFile(filepath.Join(dir, ".env"))
	if err != nil {
		t.Fatalf("read golden env: %v (run with -update to create)", err)
	}
	return compose, env
}

// envKeys lists .env keys only — never secret values, for failure output.
func envKeys(env string) []string {
	var keys []string
	for _, line := range strings.Split(env, "\n") {
		if line == "" {
			continue
		}
		k, _, _ := strings.Cut(line, "=")
		keys = append(keys, k)
	}
	return keys
}

func TestGoldenCompareAll(t *testing.T) {
	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) {
			compose, env, err := Generate(g.spec, g.creds, g.opts)
			if err != nil {
				t.Fatalf("Generate(): %v", err)
			}
			wantCompose, wantEnv := requireGolden(t, g.dir)
			if !bytes.Equal(compose, wantCompose) {
				t.Fatalf("compose mismatch for %s\ngot:\n%s\n\nwant:\n%s", g.name, compose, wantCompose)
			}
			if !bytes.Equal(env, wantEnv) {
				t.Fatalf("env mismatch for %s\ngot keys: %v\nwant keys: %v", g.name, envKeys(string(env)), envKeys(string(wantEnv)))
			}
		})
	}
}

func TestDeterministicSixApps(t *testing.T) {
	shuffled := []AppSpec{{Name: "seerr"}, {Name: "jellyfin"}, {Name: "qbittorrent"}, {Name: "prowlarr"}, {Name: "radarr"}, {Name: "sonarr"}}
	a, _, err := Generate(familySpec, familyCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	b, _, err := Generate(shuffled, familyCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !bytes.Equal(a, b) {
		t.Fatalf("spec order changed output")
	}
	c, _, err := Generate(familySpec, familyCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !bytes.Equal(a, c) {
		t.Fatal("repeat Generate() with identical inputs differs")
	}
}

func TestImagesPinned(t *testing.T) {
	compose, _, err := Generate(familySpec, familyCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	for _, line := range strings.Split(string(compose), "\n") {
		if !strings.HasPrefix(strings.TrimSpace(line), "image: ") {
			continue
		}
		ref := strings.TrimSpace(line)[len("image: "):]
		if strings.Contains(ref, ":latest") {
			t.Fatalf("image %q uses forbidden :latest tag", ref)
		}
		at := strings.Index(ref, "@")
		colon := strings.LastIndex(ref, ":")
		pinned := at > 0 || (colon > 0 && ref[colon+1:] != "")
		if !pinned {
			t.Errorf("image %q is not pinned (tag or digest)", ref)
		}
	}
}

func TestComposeUserAppsNoPuidEnv(t *testing.T) {
	compose, _, err := Generate([]AppSpec{{Name: "jellyfin"}, {Name: "seerr"}}, familyCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	for _, line := range strings.Split(string(compose), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PUID:") || strings.HasPrefix(trimmed, "PGID:") {
			t.Errorf("compose_user app renders env %q", trimmed)
		}
	}
}

func TestDockerComposeConfig(t *testing.T) {
	for _, g := range goldens {
		t.Run(g.name, func(t *testing.T) {
			if _, err := exec.LookPath("docker"); err != nil {
				if os.Getenv("CI") == "true" {
					t.Fatal("docker required in CI but not available")
				}
				t.Skip("docker not available locally")
			}
			compose, env, err := Generate(g.spec, g.creds, g.opts)
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
		})
	}
}

func TestWindowsPaths(t *testing.T) {
	compose, env, err := Generate(familySpec, familyCreds, winOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	// Windows: no PUID/PGID/user: anywhere in compose.
	for _, line := range strings.Split(string(compose), "\n") {
		trimmed := strings.TrimSpace(line)
		if strings.HasPrefix(trimmed, "PUID:") || strings.HasPrefix(trimmed, "PGID:") {
			t.Errorf("windows compose renders %q", trimmed)
		}
		if strings.Contains(trimmed, "user:") {
			t.Errorf("windows compose renders %q", trimmed)
		}
	}
	if strings.Contains(string(compose), "UMASK") {
		t.Errorf("windows compose must not set UMASK:\n%s", compose)
	}
	// Normalized forward-slash bind mounts.
	if !strings.Contains(string(compose), "C:/Media/config/qbittorrent:/config") {
		t.Errorf("compose missing normalized windows config volume:\n%s", compose)
	}
	if !strings.Contains(string(compose), "C:/Media/data:/data") {
		t.Errorf("compose missing normalized windows data volume:\n%s", compose)
	}
	// TZ IS rendered on windows (answers.yaml timezone); PUID/PGID are not.
	envStr := string(env)
	if !strings.Contains(envStr, "TZ=UTC") {
		t.Errorf("windows .env missing TZ=UTC:\n%s", envStr)
	}
	for _, key := range []string{"PUID=", "PGID=", "UMASK="} {
		if strings.Contains(envStr, key) {
			t.Errorf(".env must not contain %q on windows:\n%s", key, envStr)
		}
	}
}

func TestPortConflictQbittorrent(t *testing.T) {
	conflict := []AppSpec{{Name: "qbittorrent", HostPortOverride: map[int]int{8080: 18080}}}
	compose, env, err := Generate(conflict, qbtCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if !strings.Contains(string(compose), " - \"18080:18080\"") {
		t.Errorf("expected equal host/container remap 18080:18080:\n%s", compose)
	}
	if !strings.Contains(string(env), "WEBUI_PORT=18080") {
		t.Errorf("expected WEBUI_PORT=18080 in .env:\n%s", env)
	}
	_, envDefault, err := Generate(qbtSpec, qbtCreds, baseOpts)
	if err != nil {
		t.Fatalf("Generate(): %v", err)
	}
	if strings.Contains(string(envDefault), "WEBUI_PORT") {
		t.Errorf("default render must not set WEBUI_PORT:\n%s", envDefault)
	}
}

func TestRootRefused(t *testing.T) {
	rootOpts := Options{MediaRoot: "/srv/media", HostOS: "linux", PUID: 0, PGID: 1000, TZ: "UTC"}
	if _, _, err := Generate(qbtSpec, qbtCreds, rootOpts); err == nil {
		t.Fatal("expected error for PUID 0 on linux")
	} else if !strings.Contains(err.Error(), "refusing to run the stack as root") {
		t.Fatalf("unexpected error: %v", err)
	}
}
