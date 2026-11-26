package templates

import (
	"strings"
	"testing"
)

// expectedApps is every v1 app with a fragment.
var expectedApps = []string{"sonarr", "radarr", "prowlarr", "qbittorrent", "jellyfin", "seerr"}

// documentedSlotValues are the slot types the generator understands (G8).
var documentedSlotValues = map[string]struct{}{
	"media-manager":   {},
	"indexer":         {},
	"download-client": {},
	"media-server":    {},
	"search-service":  {},
}

// TestFragmentsParse ensures every v1 app has an embedded fragment that parses
// and whose image key resolves against versions.yaml without "latest" (G1, G5).
func TestFragmentsParse(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	for _, app := range expectedApps {
		f, err := LoadFragment(app)
		if err != nil {
			t.Fatalf("LoadFragment(%q): %v", app, err)
		}
		if f.App != app {
			t.Fatalf("LoadFragment(%q).App = %q, want %q", app, f.App, app)
		}
		ref, err := Image(f.ImageKey)
		if err != nil {
			t.Fatalf("fragment %q image key %q: %v", app, f.ImageKey, err)
		}
		if strings.EqualFold(ref, ":latest") || strings.HasSuffix(ref, ":latest") {
			t.Fatalf("fragment %q resolved image %q uses forbidden 'latest' tag", app, ref)
		}
		_, ok := documentedSlotValues[f.Slot]
		if !ok {
			t.Fatalf("fragment %q slot %q not in documented slot list", app, f.Slot)
		}
		for _, e := range f.Environment {
			if e.Value != "" && !strings.HasPrefix(e.Value, "${") {
				t.Fatalf("fragment %q env %q has literal value %q; only ${VAR} refs allowed", app, e.Name, e.Value)
			}
		}
	}
}

// expectedPorts is the contract the generator must honor for default host ports.
var expectedPorts = map[string]map[int]int{ // container -> host
	"sonarr":      {8989: 8989},
	"radarr":      {7878: 7878},
	"prowlarr":    {9696: 9696},
	"qbittorrent": {8080: 8080},
	"jellyfin":    {8096: 8096},
	"seerr":       {5055: 5055},
}

// expectedHealthcheckTool is the HTTP probe tool each pinned image provides
// (verified against the real image): curl for all except seerr (wget).
var expectedHealthcheckTool = map[string]string{
	"sonarr":      "curl",
	"radarr":      "curl",
	"prowlarr":    "curl",
	"qbittorrent": "curl",
	"jellyfin":    "curl",
	"seerr":       "wget",
}

// TestExpectedPorts is an explicit ports table asserted against fragments.
func TestExpectedPorts(t *testing.T) {
	for app, want := range expectedPorts {
		f, err := LoadFragment(app)
		if err != nil {
			t.Fatalf("LoadFragment(%q): %v", app, err)
		}
		got := map[int]int{}
		for _, p := range f.Ports {
			got[p.Container] = p.Host
		}
		for c, h := range want {
			if got[c] != h {
				t.Fatalf("fragment %q port container %d host %d, want %d", app, c, got[c], h)
			}
		}
	}
}

// expectedMounts is the explicit least-privilege mount table per app:
//
//	app            container path(s)                          mode      data mount?
//	sonarr         /config, /data                             rw        yes (rw)
//	radarr         /config, /data                             rw        yes (rw)
//	qbittorrent    /config, /data                             rw        yes (rw)
//	prowlarr       /config                                    —         no  (config only)
//	jellyfin       /config, /data                             ro for /data yes (ro)
//	seerr          /app/config                                —         no  (config only)
var expectedMounts = map[string]struct {
	volumes []Volume // parsed expected
}{
	"sonarr":      {volumes: []Volume{{Host: "<root>/config/sonarr", Container: "/config"}, {Host: "<root>/data", Container: "/data"}}},
	"radarr":      {volumes: []Volume{{Host: "<root>/config/radarr", Container: "/config"}, {Host: "<root>/data", Container: "/data"}}},
	"qbittorrent": {volumes: []Volume{{Host: "<root>/config/qbittorrent", Container: "/config"}, {Host: "<root>/data", Container: "/data"}}},
	"prowlarr":    {volumes: []Volume{{Host: "<root>/config/prowlarr", Container: "/config"}}},
	"jellyfin":    {volumes: []Volume{{Host: "<root>/config/jellyfin", Container: "/config"}, {Host: "<root>/config/jellyfin-cache", Container: "/cache"}, {Host: "<root>/data", Container: "/data", ReadOnly: true}}},
	"seerr":       {volumes: []Volume{{Host: "<root>/config/seerr", Container: "/app/config"}}},
}

// TestExpectedMounts is an explicit mounts table asserted against fragments.
func TestExpectedMounts(t *testing.T) {
	for app, want := range expectedMounts {
		f, err := LoadFragment(app)
		if err != nil {
			t.Fatalf("LoadFragment(%q): %v", app, err)
		}
		if len(f.Volumes) != len(want.volumes) {
			t.Fatalf("fragment %q: %d volumes, want %d", app, len(f.Volumes), len(want.volumes))
		}
		for i, wv := range want.volumes {
			v := f.Volumes[i]
			if v.Host != wv.Host || v.Container != wv.Container || v.ReadOnly != wv.ReadOnly {
				t.Errorf("fragment %q volume[%d] = {%q -> %q ro=%v}, want {%q -> %q ro=%v}", app, i, v.Host, v.Container, v.ReadOnly, wv.Host, wv.Container, wv.ReadOnly)
			}
		}
	}
}

// TestExpectedHealthcheckTool asserts each fragment's healthcheck uses a tool
// that exists in its pinned image (probe: curl in sonarr/radarr/prowlarr/
// qbittorrent/jellyfin, wget in seerr/seerr:v3.5.2).
func TestExpectedHealthcheckTool(t *testing.T) {
	for app, want := range expectedHealthcheckTool {
		f, err := LoadFragment(app)
		if err != nil {
			t.Fatalf("LoadFragment(%q): %v", app, err)
		}
		if f.Healthcheck == nil || len(f.Healthcheck.Test) < 2 {
			t.Fatalf("fragment %q: healthcheck missing or malformed", app)
		}
		if got := f.Healthcheck.Test[1]; got != want {
			t.Fatalf("fragment %q healthcheck tool %q, want %q", app, got, want)
		}
	}
}

// TestValidate runs Validate() and asserts seerr's pinned digest (G5).
func TestValidate(t *testing.T) {
	if err := Validate(); err != nil {
		t.Fatalf("Validate(): %v", err)
	}
	pin, ok := Pins["seerr"]
	if !ok {
		t.Fatalf("Validate(): seerr pin missing from versions.yaml")
	}
	want := "sha256:aadee014b40e28cf9aa4928bc9d1183a6eb08f8c44d543cddc9efdcb16549be2"
	if pin.Digest != want {
		t.Fatalf("seerr digest = %q, want %q", pin.Digest, want)
	}
}

// TestComposeUserAppsHaveNoPuidEnv asserts compose_user apps render no
// PUID/PGID/TZ environment entries (the jellyfin image ignores them; the
// generator sets compose user: instead). Fixed by spike S5.
func TestComposeUserAppsHaveNoPuidEnv(t *testing.T) {
	for _, app := range []string{"jellyfin", "seerr"} {
		f, err := LoadFragment(app)
		if err != nil {
			t.Fatalf("LoadFragment(%q): %v", app, err)
		}
		if !f.UserRules.ComposeUser {
			t.Fatalf("fragment %q: expected compose_user", app)
		}
		for _, e := range f.Environment {
			switch e.Name {
			case "PUID", "PGID", "TZ":
				t.Fatalf("fragment %q: compose_user app must not set env %q", app, e.Name)
			}
		}
	}
}
