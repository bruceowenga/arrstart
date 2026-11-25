// Package generator renders per-app templates into docker-compose.yml + .env.
//
// Render-only: Generate() produces bytes; it performs no file writes and no
// Docker calls. Output is deterministic (sorted keys, stable ordering).
package generator

import (
	"fmt"
	"sort"
	"strings"

	"github.com/bruceowenga/arrstart/templates"
)

// AppSpec describes one app to render into docker-compose.yml.
type AppSpec struct {
	Name string // app name: sonarr, radarr, prowlarr, qbittorrent, jellyfin, seerr

	// HostPortOverride maps container port -> requested host port, per app.
	// Zero value means: use the fragment's declared default host port.
	HostPortOverride map[int]int
}

// Options carries per-invocation render settings.
type Options struct {
	MediaRoot string // resolved host media root, e.g. "/srv/media" or "C:\Media"
	HostOS    string // linux|windows|darwin
	PUID      int    // invoking user's UID (0 when unset or windows)
	PGID      int    // invoking user's GID (0 when unset or windows)
	TZ        string // timezone
}

// Generate renders compose bytes + .env bytes from specs and credentials.
//
// It calls templates.Validate() first and returns its error, so Image() can
// resolve pinned refs. Image refs are "image:tag" or "image@digest" when a
// digest exists. Host/container ports stay equal; on a host-port conflict the
// generator maps P:P and injects WEBUI_PORT=P. Bind-mount paths are built from
// structured segments (never <root> string substitution; Windows normalized to
// C:/Media style). LinuxServer PUID/PGID/TZ env only for puid_pgid apps; compose
// "user:" only for compose_user apps; Windows gets neither. Healthcheck comes
// from the fragment (never hard-coded).
func Generate(specs []AppSpec, creds map[string]string, opts Options) (compose, env []byte, err error) {
	if err := templates.Validate(); err != nil {
		return nil, nil, err
	}

	cb := &composeBuilder{opts: opts}
	cb.b.WriteString("# arrstart generated compose — hand-editable. Never use the latest tag.\nservices:\n")
	for _, spec := range specs {
		f, err := templates.LoadFragment(spec.Name)
		if err != nil {
			return nil, nil, err
		}
		if err = cb.renderService(spec, f); err != nil {
			return nil, nil, err
		}
	}

	envVars := envLines(opts, creds, cb.webUIPort)
	keys := make([]string, 0, len(envVars))
	for k := range envVars {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var envB strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&envB, "%s=%s\n", k, envVars[k])
	}

	return []byte(strings.TrimRight(cb.b.String(), "\n")), []byte(strings.TrimRight(envB.String(), "\n")), nil
}

// composeBuilder accumulates one docker-compose.yml document.
type composeBuilder struct {
	opts      Options
	b         strings.Builder
	webUIPort string // set when any port is remapped (host != container)
}

func (cb *composeBuilder) renderService(spec AppSpec, f *templates.Fragment) error {
	image, err := templates.Image(f.ImageKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(&cb.b, "  %s:\n    image: %s\n", f.App, image)

	if len(f.Ports) > 0 {
		cb.b.WriteString("    ports:\n")
		for _, p := range f.Ports {
			host := p.Host
			if ov, ok := spec.HostPortOverride[p.Container]; ok && ov != 0 {
				host = ov
			}
			if host != p.Container {
				cb.webUIPort = fmt.Sprintf("%d", host)
			}
			fmt.Fprintf(&cb.b, "      - \"%d:%d\"\n", host, host)
		}
	}

	if len(f.Volumes) > 0 {
		cb.b.WriteString("    volumes:\n")
		for _, v := range f.Volumes {
			host := rootPath(cb.opts, v.Host)
			ro := ""
			if v.ReadOnly {
				ro = ":ro"
			}
			fmt.Fprintf(&cb.b, "      - %q:%s%s\n", host, v.Container, ro)
		}
	}

	envAdded := make(map[string]struct{}, len(f.Environment))
	if len(f.Environment) > 0 {
		cb.b.WriteString("    environment:\n")
		for _, e := range f.Environment {
			envAdded[e.Name] = struct{}{}
			fmt.Fprintf(&cb.b, "      %s: %s\n", e.Name, e.Value)
		}
	}

	linux := cb.opts.HostOS != "windows"
	if f.UserRules.PUIDPGID && linux {
		if _, ok := envAdded["PUID"]; !ok {
			fmt.Fprintf(&cb.b, "      PUID: \"%d\"\n", cb.opts.PUID)
		}
		if _, ok := envAdded["PGID"]; !ok {
			fmt.Fprintf(&cb.b, "      PGID: \"%d\"\n", cb.opts.PGID)
		}
		if _, ok := envAdded["TZ"]; !ok && cb.opts.TZ != "" {
			fmt.Fprintf(&cb.b, "      TZ: \"%s\"\n", cb.opts.TZ)
		}
		if _, ok := envAdded["UMASK"]; !ok {
			fmt.Fprintf(&cb.b, "      UMASK: \"002\"\n")
		}
	}

	if f.UserRules.ComposeUser && linux {
		fmt.Fprintf(&cb.b, "    user: \"${PUID}:${PGID}\"\n")
	}

	if f.Healthcheck != nil && len(f.Healthcheck.Test) > 0 {
		cb.b.WriteString("    healthcheck:\n")
		fmt.Fprintf(&cb.b, "      test: %s\n", renderTest(f.Healthcheck.Test))
		fmt.Fprintf(&cb.b, "      interval: %s\n", f.Healthcheck.Interval)
		fmt.Fprintf(&cb.b, "      timeout: %s\n", f.Healthcheck.Timeout)
		fmt.Fprintf(&cb.b, "      retries: %d\n", f.Healthcheck.Retries)
	}

	return nil
}

// renderTest renders a healthcheck test array as compose YAML.
func renderTest(test []string) string {
	var b strings.Builder
	b.WriteString("[")
	for i, t := range test {
		if i > 0 {
			b.WriteString(", ")
		}
		fmt.Fprintf(&b, "%q", t)
	}
	b.WriteString("]")
	return b.String()
}

// rootPath builds a bind-mount host path from structured segments: the leading
// "<root>" segment is replaced by opts.MediaRoot (backslashes normalized to
// forward slashes on Windows). No string substitution of "<root>" elsewhere.
func rootPath(opts Options, p string) string {
	root := strings.ReplaceAll(opts.MediaRoot, "\\", "/")
	parts := strings.Split(p, "/")
	if len(parts) > 0 && parts[0] == "<root>" {
		parts[0] = root
	}
	return strings.Join(parts, "/")
}

// envLines builds .env entries from rendered state: credentials plus PUID/PGID/TZ
// on Linux and WEBUI_PORT when a port was remapped. Caller sorts keys.
func envLines(opts Options, creds map[string]string, webUIPort string) map[string]string {
	m := make(map[string]string, len(creds)+4)
	for k, v := range creds {
		m[k] = v
	}
	if opts.HostOS != "windows" {
		if opts.PUID != 0 {
			m["PUID"] = fmt.Sprintf("%d", opts.PUID)
		}
		if opts.PGID != 0 {
			m["PGID"] = fmt.Sprintf("%d", opts.PGID)
		}
		if opts.TZ != "" {
			m["TZ"] = opts.TZ
		}
	}
	if webUIPort != "" {
		m["WEBUI_PORT"] = webUIPort
	}
	return m
}
