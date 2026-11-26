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

	linux := opts.HostOS != "windows"
	if linux && (opts.PUID == 0 || opts.PGID == 0) {
		return nil, nil, fmt.Errorf("refusing to run the stack as root; set puid and pgid in answers.yaml (or run as a normal user)")
	}

	// Sort specs for deterministic, stable service ordering regardless of input
	// order (step-1: deterministic output).
	ordered := make([]AppSpec, len(specs))
	copy(ordered, specs)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i].Name < ordered[j].Name })

	cb := &composeBuilder{opts: opts, creds: creds}
	cb.b.WriteString("# arrstart generated compose — hand-editable. Never use the latest tag.\nservices:\n")
	for _, spec := range ordered {
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

	composeOut := strings.TrimRight(cb.b.String(), "\n") + "\n"
	envOut := strings.TrimRight(envB.String(), "\n") + "\n"
	return []byte(composeOut), []byte(envOut), nil
}

// servarrAPIKey maps a Servarr app to the compose env var that carries its
// API key and the matching credential key. Rendered only when the credential
// key is present (spike S1, verified against pinned containers).
var servarrAPIKey = map[string]struct {
	env  string // compose environment variable name
	cred string // credential key in the credentials map
}{
	"sonarr":   {env: "SONARR__AUTH__APIKEY", cred: "SONARR_APIKEY"},
	"radarr":   {env: "RADARR__AUTH__APIKEY", cred: "RADARR_APIKEY"},
	"prowlarr": {env: "PROWLARR__AUTH__APIKEY", cred: "PROWLARR_APIKEY"},
}

// composeBuilder accumulates one docker-compose.yml document.
type composeBuilder struct {
	opts      Options
	creds     map[string]string // credentials; compose references them only as ${VAR}
	b         strings.Builder
	webUIPort string // set when any port is remapped (host != container)
}

func (cb *composeBuilder) renderService(spec AppSpec, f *templates.Fragment) error {
	image, err := templates.Image(f.ImageKey)
	if err != nil {
		return err
	}
	fmt.Fprintf(&cb.b, "  %s:\n    image: %s\n    restart: unless-stopped\n", f.App, image)

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
			mount := host + ":" + v.Container + ro
			fmt.Fprintf(&cb.b, "      - %q\n", mount)
		}
	}

	// One ordered environment list: fragment entries first, then rules.
	// PUID/PGID are LinuxServer-only: skipped on windows and for
	// compose_user apps. TZ is honoured on every OS for non-compose_user
	// apps only (never verified for Jellyfin/Seerr images).
	type envEntry struct{ name, value string }
	var envList []envEntry
	envAdded := make(map[string]struct{}, len(f.Environment))
	add := func(name, value string) {
		if _, dup := envAdded[name]; dup {
			return
		}
		envAdded[name] = struct{}{}
		envList = append(envList, envEntry{name, value})
	}
	linux := cb.opts.HostOS != "windows"
	for _, e := range f.Environment {
		if (e.Name == "PUID" || e.Name == "PGID") && (!linux || f.UserRules.ComposeUser) {
			continue
		}
		add(e.Name, e.Value)
	}
	if f.UserRules.PUIDPGID && linux {
		add("PUID", "${PUID}")
		add("PGID", "${PGID}")
	}
	if cb.opts.TZ != "" && !f.UserRules.ComposeUser {
		add("TZ", "${TZ}")
	}
	// Inject the Servarr API key env var when a matching credential exists
	// (spike S1: SONARR__AUTH__APIKEY etc., verified against pinned containers).
	// The value itself lives only in .env; compose references ${VAR} only.
	if m, ok := servarrAPIKey[f.App]; ok {
		if _, hasCred := cb.creds[m.cred]; hasCred {
			add(m.env, "${"+m.cred+"}")
		}
	}
	if len(envList) > 0 {
		cb.b.WriteString("    environment:\n")
		for _, e := range envList {
			fmt.Fprintf(&cb.b, "      %s: %q\n", e.name, e.value)
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
	}
	if opts.TZ != "" {
		m["TZ"] = opts.TZ
	}
	if webUIPort != "" {
		m["WEBUI_PORT"] = webUIPort
	}
	return m
}
