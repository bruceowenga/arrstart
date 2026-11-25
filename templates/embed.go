// Package templates embeds per-app compose fragments and pinned image refs.
//
// All *.yaml in this directory are embedded via go:embed:
//
//	versions.yaml   — single source of truth for image:tag pins (G5, never "latest")
//	<app>.yaml      — per-app compose fragment(s) (G1), one per v1 app
//
// TemplatesFS exposes the embedded files; LoadFragment and Image are the typed
// accessors consumed by internal/generator.
package templates

import (
	"embed"
	"fmt"
	"io/fs"
	"strings"

	"gopkg.in/yaml.v3"
)

//go:embed *.yaml
var TemplatesFS embed.FS

// Fragment is one per-app compose fragment (G1). It describes how a single
// app is rendered into docker-compose.yml; policy (paths, PUID/PGID,
// port fallback) is applied by internal/generator, not hard-coded here (G8).
type Fragment struct {
	App           string       `yaml:"app"`
	Slot          string       `yaml:"slot"`  // "media-server" for jellyfin; other slots picked per app
	ImageKey      string       `yaml:"image"` // key into versions.yaml images map; resolved to image:tag by Image()
	Ports         []Port       `yaml:"ports"`
	Volumes       []Volume     `yaml:"volumes"`
	Environment   []EnvVar     `yaml:"environment"`
	Healthcheck   *Healthcheck `yaml:"healthcheck"`
	RamEstimateMB int          `yaml:"ram_estimate_mb"`
	UserRules     UserRules    `yaml:"user_rules"`
}

// Port maps one container port to a default host port.
type Port struct {
	Container int `yaml:"container"` // container port
	Host      int `yaml:"host"`      // default host port; 0 => generator picks a free host port (G6)
}

// Volume is one bind mount. Host may contain the "<root>" placeholder that the
// generator replaces with media_root (normalized for Windows, G3).
type Volume struct {
	Host      string `yaml:"host"`      // e.g. "<root>/config/sonarr"
	Container string `yaml:"container"` // e.g. /config
	ReadOnly  bool   `yaml:"read_only"` // true => generator emits :ro
}

// EnvVar is one environment variable injected into the container.
type EnvVar struct {
	Name  string `yaml:"name"`
	Value string `yaml:"value"` // literal or ${VAR} reference into .env
}

// Healthcheck describes the container health check rendered into compose.
type Healthcheck struct {
	Test     []string `yaml:"test"`     // e.g. ["CMD", "curl", "-f", "http://localhost:8989/"]
	Interval string   `yaml:"interval"` // e.g. 30s
	Timeout  string   `yaml:"timeout"`  // e.g. 10s
	Retries  int      `yaml:"retries"`  // e.g. 3
}

// UserRules govern OS-specific runtime behaviour (G3, G4).
type UserRules struct {
	OS          []string `yaml:"os"`           // linux|darwin|windows when these rules apply
	PUIDPGID    bool     `yaml:"puid_pgid"`    // inject PUID/PGID/TZ env (LSIO convention)
	ComposeUser bool     `yaml:"compose_user"` // set compose user: "PUID:PGID" (seerr convention)
}

// Pin is one pinned image:tag from versions.yaml (G5: tag is exact, never "latest").
type Pin struct {
	Image string `yaml:"image"`
	Tag   string `yaml:"tag"`
}

// Pins holds the parsed versions.yaml images map, populated at init from the
// embedded versions.yaml. Keyed by app name.
var Pins map[string]Pin

func init() {
	Pins = make(map[string]Pin)
	data, err := fs.ReadFile(TemplatesFS, "versions.yaml")
	if err != nil {
		panic(fmt.Sprintf("templates: read embedded versions.yaml: %v", err))
	}
	var doc struct {
		Images map[string]Pin `yaml:"images"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		panic(fmt.Sprintf("templates: parse embedded versions.yaml: %v", err))
	}
	for app, pin := range doc.Images {
		if pin.Tag == "" {
			panic(fmt.Sprintf("templates: %s has empty tag in versions.yaml", app))
		}
		if strings.EqualFold(pin.Tag, "latest") {
			panic(fmt.Sprintf("templates: %s uses forbidden 'latest' tag in versions.yaml", app))
		}
		Pins[app] = pin
	}
}

// Image returns the pinned "image:tag" ref for the given app key (G5). The
// boolean is false when the key is absent from versions.yaml.
func Image(key string) (ref string, ok bool) {
	pin, ok := Pins[key]
	if !ok {
		return "", false
	}
	return pin.Image + ":" + pin.Tag, true
}

// LoadFragment returns the per-app fragment for the given app (G1).
func LoadFragment(app string) (*Fragment, error) {
	data, err := fs.ReadFile(TemplatesFS, app+".yaml")
	if err != nil {
		return nil, fmt.Errorf("templates: load fragment %q: %w", app, err)
	}
	var f Fragment
	if err := yaml.Unmarshal(data, &f); err != nil {
		return nil, fmt.Errorf("templates: parse fragment %q: %w", app, err)
	}
	if f.App == "" {
		f.App = app
	}
	if f.ImageKey == "" {
		f.ImageKey = app
	}
	return &f, nil
}
