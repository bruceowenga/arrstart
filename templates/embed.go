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
	"regexp"
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
	Image  string `yaml:"image"`
	Tag    string `yaml:"tag"`
	Digest string `yaml:"digest,omitempty"` // optional sha256:<64 hex>; verified by Validate()
}

// Pins holds the parsed versions.yaml images map, populated by Validate() and
// keyed by app name.
var Pins map[string]Pin

// digestRe validates an optional image digest: sha256:<64 lowercase hex>.
var digestRe = regexp.MustCompile(`^sha256:[0-9a-f]{64}$`)

// init populates Pins from the embedded versions.yaml without panicking;
// validation errors surface through Validate() instead.
func init() {
	_ = Validate()
}

// Validate parses the embedded versions.yaml and returns an error if any pin
// is missing a tag, uses "latest", or carries a malformed digest. On success
// it (re)populates Pins. Safe to call multiple times; consumed by the
// generator at startup and by templates_test.go (G1, G5).
func Validate() error {
	data, err := fs.ReadFile(TemplatesFS, "versions.yaml")
	if err != nil {
		return fmt.Errorf("templates: read embedded versions.yaml: %w", err)
	}
	var doc struct {
		Images map[string]Pin `yaml:"images"`
	}
	if err := yaml.Unmarshal(data, &doc); err != nil {
		return fmt.Errorf("templates: parse embedded versions.yaml: %w", err)
	}
	pins := make(map[string]Pin, len(doc.Images))
	for app, pin := range doc.Images {
		if pin.Tag == "" {
			return fmt.Errorf("templates: %s has empty tag in versions.yaml", app)
		}
		if strings.EqualFold(pin.Tag, "latest") {
			return fmt.Errorf("templates: %s uses forbidden 'latest' tag in versions.yaml", app)
		}
		if pin.Digest != "" && !digestRe.MatchString(pin.Digest) {
			return fmt.Errorf("templates: %s has malformed digest %q; want sha256:<64 hex", app, pin.Digest)
		}
		pins[app] = pin
	}
	Pins = pins
	return nil
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
