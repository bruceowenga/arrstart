package apply

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeAnswers(t *testing.T, dir, content string) string {
	t.Helper()
	path := filepath.Join(dir, "answers.yaml")
	if err := os.WriteFile(path, []byte(content), 0o600); err != nil {
		t.Fatalf("write answers fixture: %v", err)
	}
	return path
}

func TestValidateAnswers(t *testing.T) {

	tests := []struct {
		name    string
		content string
		hostOS  string // override host_os; "" means runtime default
		wantErr string // substring of expected error, or "" for success
	}{
		{
			name: "movies preset linux valid",
			content: `version: 1
preset: movies
media_root: /data
`,
			wantErr: "",
		},
		{
			name: "family preset linux valid",
			content: `version: 1
preset: family
media_root: /data
`,
			wantErr: "",
		},
		{
			name: "custom preset with apps valid",
			content: `version: 1
preset: custom
apps:
  - sonarr
  - radarr
media_root: /data
`,
			wantErr: "",
		},
		{
			name: "windows media root backslash valid",
			content: `version: 1
preset: movies
media_root: C:\Media
host_os: windows
`,
			wantErr: "",
		},
		{
			name: "windows media root forward slash valid",
			content: `version: 1
preset: movies
media_root: C:/Media
host_os: windows
`,
			wantErr: "",
		},
		{
			name: "windows UNC path valid",
			content: `version: 1
preset: movies
media_root: \\server\share
host_os: windows
`,
			wantErr: "",
		},
		{
			name: "with puid pgid on linux valid",
			content: `version: 1
preset: movies
media_root: /data
puid: 1000
pgid: 1000
`,
			wantErr: "",
		},
		{
			name: "with puid pgid on darwin valid",
			content: `version: 1
preset: movies
media_root: /data
host_os: darwin
puid: 501
pgid: 20
`,
			wantErr: "",
		},
		{
			name: "extras all false valid",
			content: `version: 1
preset: movies
media_root: /data
extras:
  vpn: false
  tailscale: false
`,
			wantErr: "",
		},
		{
			name: "version not 1 invalid",
			content: `version: 2
preset: movies
media_root: /data
`,
			wantErr: "version: expected 1, got 2",
		},
		{
			name: "unknown preset invalid",
			content: `version: 1
preset: ultimate
media_root: /data
`,
			wantErr: "preset: \"ultimate\" is not yet supported",
		},
		{
			name: "custom preset without apps invalid",
			content: `version: 1
preset: custom
media_root: /data
`,
			wantErr: "apps: required when preset is custom",
		},
		{
			name: "media root empty invalid",
			content: `version: 1
preset: movies
`,
			wantErr: "media_root: required",
		},
		{
			name: "media root relative on linux invalid",
			content: `version: 1
preset: movies
media_root: relative/path
host_os: linux
`,
			wantErr: "media_root: \"relative/path\" is not an absolute path",
		},
		{
			name: "media root relative on linux shows resolved host_os",
			content: `version: 1
preset: movies
media_root: relative/path
host_os: linux
`,
			wantErr: `media_root: "relative/path" is not an absolute path for host_os "linux"`,
		},
		{
			name: "media root relative on windows invalid",
			content: `version: 1
preset: movies
media_root: relative/path
host_os: windows
`,
			wantErr: "media_root: \"relative/path\" is not an absolute path",
		},
		{
			name: "media root windows slash missing invalid",
			content: `version: 1
preset: movies
media_root: Media
host_os: windows
`,
			wantErr: "media_root: \"Media\" is not an absolute path",
		},
		{
			name: "host_os invalid invalid",
			content: `version: 1
preset: movies
media_root: /data
host_os: freebsd
`,
			wantErr: "host_os: \"freebsd\" is not supported",
		},
		{
			name: "puid on windows invalid",
			content: `version: 1
preset: movies
media_root: C:\Media
host_os: windows
puid: 1000
`,
			wantErr: "puid: not supported on windows",
		},
		{
			name: "pgid on windows invalid",
			content: `version: 1
preset: movies
media_root: C:\Media
host_os: windows
pgid: 1000
`,
			wantErr: "pgid: not supported on windows",
		},
		{
			name: "windows media_root accepted on any host OS",
			content: `version: 1
preset: movies
media_root: C:\Media
host_os: windows
`,
			wantErr: "",
		},
		{
			name: "windows rejects puid",
			content: `version: 1
preset: movies
media_root: C:\Media
host_os: windows
puid: 1000
`,
			wantErr: "puid",
		},
		{
			name: "extras vpn true invalid",
			content: `version: 1
preset: movies
media_root: /data
extras:
  vpn: true
  tailscale: false
`,
			wantErr: "extras: vpn and tailscale must be false in v1",
		},
		{
			name: "extras tailscale true invalid",
			content: `version: 1
preset: movies
media_root: /data
extras:
  vpn: false
  tailscale: true
`,
			wantErr: "extras: vpn and tailscale must be false in v1",
		},
		{
			name: "unknown app invalid",
			content: `version: 1
preset: custom
apps:
  - mythical-app
media_root: /data
`,
			wantErr: "apps: \"mythical-app\" is not a known app",
		},
		{
			name: "duplicate app invalid",
			content: `version: 1
preset: custom
apps:
  - sonarr
  - sonarr
media_root: /data
`,
			wantErr: "apps: duplicate entry \"sonarr\"",
		},
		{
			name: "unknown field rejected",
			content: `version: 1
preset: movies
media_root: /data
bogus_field: value
`,
			wantErr: "decode answers",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			dir := t.TempDir()
			path := writeAnswers(t, dir, tt.content)

			// Override host_os if specified - rewrite the file with host_os set
			if tt.hostOS != "" {
				content := tt.content
				// Insert host_os after media_root line
				lines := strings.Split(content, "\n")
				var newLines []string
				for _, line := range lines {
					newLines = append(newLines, line)
					if strings.HasPrefix(strings.TrimSpace(line), "media_root:") {
						newLines = append(newLines, "host_os: "+tt.hostOS)
					}
				}
				content = strings.Join(newLines, "\n")
				path = writeAnswers(t, dir, content)
			}

			a, err := LoadAnswers(path)
			if tt.wantErr == "" {
				if err != nil {
					t.Fatalf("expected no error, got: %v", err)
				}
				if err := Validate(a); err != nil {
					t.Fatalf("Validate: expected no error, got: %v", err)
				}
				return
			}

			// Error may come from Load (strict decode) or Validate
			a, loadErr := LoadAnswers(path)
			if loadErr != nil {
				if !strings.Contains(loadErr.Error(), tt.wantErr) {
					t.Fatalf("LoadAnswers: expected error containing %q, got %v", tt.wantErr, loadErr)
				}
				return
			}
			if err := Validate(a); err != nil {
				if !strings.Contains(err.Error(), tt.wantErr) {
					t.Fatalf("Validate: expected error containing %q, got %v", tt.wantErr, err)
				}
				return
			}
			t.Fatal("expected validation error, got none")
		})
	}
}

func TestResolve(t *testing.T) {
	t.Run("movies preset expands apps", func(t *testing.T) {
		a := &Answers{
			Version:   1,
			Preset:    "movies",
			MediaRoot: "/data",
		}
		p, err := Resolve(a)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		want := []string{"radarr", "prowlarr", "qbittorrent", "jellyfin", "seerr"}
		if len(p.Apps) != len(want) {
			t.Fatalf("Resolve: got %d apps, want %d", len(p.Apps), len(want))
		}
		for i, app := range want {
			if p.Apps[i] != app {
				t.Fatalf("Resolve: app[%d] = %q, want %q", i, p.Apps[i], app)
			}
		}
		if p.MediaRoot != "/data" {
			t.Fatalf("Resolve: media_root = %q, want %q", p.MediaRoot, "/data")
		}
		if p.HostOS != DefaultHostOS() {
			t.Fatalf("Resolve: host_os = %q, want %q", p.HostOS, DefaultHostOS())
		}
		if p.Timezone != "UTC" {
			t.Fatalf("Resolve: timezone = %q, want %q", p.Timezone, "UTC")
		}
	})

	t.Run("family preset expands apps", func(t *testing.T) {
		a := &Answers{
			Version:   1,
			Preset:    "family",
			MediaRoot: "/data",
		}
		p, err := Resolve(a)
		if err != nil {
			t.Fatalf("Resolve: %v", err)
		}
		want := []string{"sonarr", "radarr", "prowlarr", "qbittorrent", "jellyfin", "seerr"}
		if len(p.Apps) != len(want) {
			t.Fatalf("Resolve: got %d apps, want %d", len(p.Apps), len(want))
		}
		for i, app := range want {
			if p.Apps[i] != app {
				t.Fatalf("Resolve: app[%d] = %q, want %q", i, p.Apps[i], app)
			}
		}
	})

	t.Run("validation error propagates", func(t *testing.T) {
		a := &Answers{Version: 99}
		if _, err := Resolve(a); err == nil {
			t.Fatal("Resolve: expected error for version 99")
		}
	})
}

// Ensure format plan is deterministic.
func TestFormatPlan(t *testing.T) {
	p := &Plan{
		Apps:      []string{"radarr", "prowlarr"},
		MediaRoot: "/data",
		HostOS:    "linux",
		Timezone:  "UTC",
		Extras:    Extras{VPN: false, Tailscale: false},
	}
	out := FormatPlan(p)
	if !strings.Contains(out, "apps: radarr, prowlarr") {
		t.Fatalf("FormatPlan missing apps: %s", out)
	}
	if !strings.Contains(out, "media_root: /data") {
		t.Fatalf("FormatPlan missing media_root: %s", out)
	}
	if !strings.Contains(out, "host_os: linux") {
		t.Fatalf("FormatPlan missing host_os: %s", out)
	}
	if !strings.Contains(out, "stages:") {
		t.Fatalf("FormatPlan missing stages: %s", out)
	}
}
