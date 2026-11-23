// Package jellyfin implements the wiring client for Jellyfin (PRD A7).
//
// WaitHealthy → Configure → Verify per internal/wiring/wiring.go.
//
// Configure completes Jellyfin's startup wizard headless via the public
// /Startup/* endpoints, creates the admin user, authenticates as that admin
// using Jellyfin's MediaBrowser authentication scheme, creates a wiring API
// key, and adds the TV (/data/media/tv) and movies (/data/media/movies)
// libraries. All calls are idempotent so Configure is safe to re-run
// (PRD A10).
//
// Jellyfin 12.4 does NOT use X-Emby-Token alone (legacy auth is off,
// requests fail with 401). Authentication is the MediaBrowser scheme:
//
//	Authorization: MediaBrowser Client="...", Device="...", DeviceId="...", Version="1.0.0.0"
//
// and, after authenticating,  ... Token="<accessToken>".
// Post-login body field names follow the served OpenAPI spec:
//
//	POST /Users/AuthenticateByName body uses "Pw", not "Password".
//	POST /Startup/User body uses "Name" and "Password".
//
// Library creation was confirmed against the real pinned container
// jellyfin/jellyfin:12.4:
//
//	POST /Library/VirtualFolders?name=<name>&collectionType=<type> with body
//	{"LibraryOptions":{"PathInfos":[{"Path":"/data/media/tv"}],...}} → 204.
//	The path must exist on disk first (Jellyfin validates it). collectionType
//	is "tvshows" or "movies". A library whose path is already present is
//	skipped so Configure is idempotent.
//
// Fields and call order below were verified against jellyfin/jellyfin:12.4:
//
//	GET    /System/Info/Public        → 200 anonymous, StartupWizardCompleted
//	POST   /Startup/Configuration     → 204
//	GET    /Startup/User              → 200 {"Name":"root"} placeholder
//	POST   /Startup/User              → 204 creates first admin user
//	POST   /Startup/Complete          → 204 flips StartupWizardCompleted true
//	POST   /Users/AuthenticateByName  → 200 AccessToken (MediaBrowser header)
//	GET    /Auth/Keys                 → items with AppName
//	POST   /Auth/Keys?app=            → 204 creates a wiring key
//	GET    /Library/VirtualFolders    → 200 libraries (name, collectionType, paths)
//	POST   /Library/VirtualFolders    → 204 add a library
//
// Live verified: wizard + authentication + key creation + library creation
// against jellyfin/jellyfin:12.4 (paths pre-created in the container).
package jellyfin

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"strings"
	"time"
)

const (
	// defaultServerName is set on the server during configuration.
	defaultServerName = "arrstart"
	// defaultAppName names the wiring API key Jellyfin issues.
	defaultAppName = "arrstart"
	// mediaBrowserClient identifies this wiring client to Jellyfin.
	mediaBrowserClient = "arrstart"
	// mediaBrowserDevice is a static device id so Jellyfin treats repeated
	// wiring runs as the same client.
	mediaBrowserDeviceID = "arrstart-wiring"
	// mediaBrowserVersion reported in the MediaBrowser header.
	mediaBrowserVersion = "1.0.0.0"
)

// Config holds the Jellyfin wiring configuration.
//
// AdminName/AdminPassword are the admin credential created by the headless
// startup wizard and used for all subsequent authenticated calls. Libraries
// are the media libraries to add; when empty the client defaults to TV and
// Movies under /data/media. ServerName is shown in the Jellyfin dashboard.
// AppName names the wiring API key Jellyfin issues (used by the Seerr
// integration and doctor connectivity checks).
type Config struct {
	AdminName     string
	AdminPassword string
	Libraries     []Library
	ServerName    string
	AppName       string
}

// Library describes one media library to add to Jellyfin.
type Library struct {
	Name           string // displayed library name, e.g. "TV Shows"
	Path           string // media path inside the container, e.g. /data/media/tv
	CollectionType string // "tvshows" or "movies"
}

// Client talks to Jellyfin's HTTP API. Configure authenticates as the admin
// user it creates and remembers the access token so Verify can re-authenticate.
type Client struct {
	baseURL        string
	adminName      string
	adminPassword  string
	appName        string
	configuredLibs []Library // libraries last configured (defaults when unset)
	token          string
	httpClient     *http.Client
}

// New returns a Jellyfin client for baseURL (e.g. http://jellyfin:8096).
func New(baseURL string) *Client {
	return &Client{
		baseURL:    strings.TrimSuffix(baseURL, "/"),
		httpClient: &http.Client{Timeout: 10 * time.Second},
	}
}

// WaitHealthy blocks until Jellyfin's API responds. GET /System/Info/Public
// is anonymous, so any HTTP status counts as healthy; connection errors are
// retried with backoff until ctx is cancelled.
func (c *Client) WaitHealthy(ctx context.Context) error {
	for {
		if resp, err := c.httpGet(ctx, "/System/Info/Public"); err == nil {
			_ = resp.Body.Close()
			return nil
		} else if resp != nil {
			_ = resp.Body.Close()
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(500 * time.Millisecond):
		}
	}
}

// Configure completes the headless wizard and syncs the admin user, API key,
// and libraries. cfg must be a *Config.
func (c *Client) Configure(ctx context.Context, cfg any) error {
	conf, ok := cfg.(*Config)
	if !ok {
		return fmt.Errorf("jellyfin.Configure: expected *Config, got %T", cfg)
	}
	if err := conf.defaults(); err != nil {
		return err
	}
	c.adminName = conf.AdminName
	c.adminPassword = conf.AdminPassword
	c.appName = conf.AppName
	c.configuredLibs = conf.Libraries

	// 1. Configure the wizard (server name). Idempotent: re-configuring is fine.
	if err := c.postJSON(ctx, "/Startup/Configuration", startupConfiguration{ServerName: conf.ServerName}, nil); err != nil {
		return fmt.Errorf("jellyfin: configure wizard: %w", err)
	}

	// 2. Create the admin user if the wizard is not yet complete, then finalise
	// the wizard. GET /Startup/User returns the "root" placeholder before the
	// first user exists (200 anonymous); after the wizard completes this
	// endpoint requires authentication, so StartupWizardCompleted drives the
	// branch instead.
	complete, err := c.wizardComplete(ctx)
	if err != nil {
		return fmt.Errorf("jellyfin: check wizard state: %w", err)
	}
	if !complete {
		placeholder, err := c.startupUserPlaceholder(ctx)
		if err != nil {
			return fmt.Errorf("jellyfin: check startup user: %w", err)
		}
		if placeholder {
			if err := c.postJSON(ctx, "/Startup/User", startupUser{Name: conf.AdminName, Password: conf.AdminPassword, HasPassword: true}, nil); err != nil {
				return fmt.Errorf("jellyfin: create admin user: %w", err)
			}
		}
		if err := c.postJSON(ctx, "/Startup/Complete", map[string]string{}, nil); err != nil {
			return fmt.Errorf("jellyfin: complete wizard: %w", err)
		}
	}

	// 3. Authenticate as the admin user. The issued access token is remembered
	// so Verify can re-authenticate without re-creating the user.
	token, err := c.authenticate(ctx, conf.AdminName, conf.AdminPassword)
	if err != nil {
		return fmt.Errorf("jellyfin: authenticate admin: %w", err)
	}
	c.token = token

	// 4. Create the wiring API key if one for this app name is not already
	// registered (idempotent).
	if present, err := c.hasAPIKey(ctx); err != nil {
		return fmt.Errorf("jellyfin: list api keys: %w", err)
	} else if !present {
		if resp, err := c.httpPost(ctx, "/Auth/Keys?app="+url.QueryEscape(conf.AppName), nil, c.mediaBrowserHeader()); err != nil {
			return fmt.Errorf("jellyfin: create api key: %w", err)
		} else {
			_ = resp.Body.Close()
		}
	}

	// 5. Sync the media libraries (idempotent: skip any whose path is present).
	if err := c.syncLibraries(ctx, conf.Libraries); err != nil {
		return fmt.Errorf("jellyfin: sync libraries: %w", err)
	}
	return nil
}

// Verify re-authenticates as the admin user and checks that the wizard is
// complete and every configured library path is present.
func (c *Client) Verify(ctx context.Context) error {
	if c.adminName == "" || c.adminPassword == "" {
		return fmt.Errorf("jellyfin.Verify: client not configured")
	}
	if _, err := c.authenticate(ctx, c.adminName, c.adminPassword); err != nil {
		return fmt.Errorf("jellyfin: authenticate admin: %w", err)
	}
	complete, err := c.wizardComplete(ctx)
	if err != nil {
		return fmt.Errorf("jellyfin: check wizard state: %w", err)
	}
	if !complete {
		return fmt.Errorf("jellyfin: startup wizard not complete")
	}
	libs, err := c.listLibraries(ctx)
	if err != nil {
		return fmt.Errorf("jellyfin: list libraries: %w", err)
	}
	have := map[string]bool{}
	for _, l := range libs {
		for _, p := range libraryPaths(l) {
			have[p] = true
		}
	}
	var missing []string
	for _, lib := range c.libraries() {
		if !have[lib.Path] {
			missing = append(missing, lib.Path)
		}
	}
	if len(missing) > 0 {
		return fmt.Errorf("jellyfin: library path(s) not registered: %s", strings.Join(missing, ", "))
	}
	return nil
}

// defaults fills in zero-valued config fields. AdminName and AdminPassword are
// required.
func (c *Config) defaults() error {
	if c.AdminName == "" {
		return fmt.Errorf("jellyfin: Config.AdminName is required")
	}
	if c.AdminPassword == "" {
		return fmt.Errorf("jellyfin: Config.AdminPassword is required")
	}
	if c.ServerName == "" {
		c.ServerName = defaultServerName
	}
	if c.AppName == "" {
		c.AppName = defaultAppName
	}
	if c.Libraries == nil {
		c.Libraries = []Library{
			{Name: "TV Shows", Path: "/data/media/tv", CollectionType: "tvshows"},
			{Name: "Movies", Path: "/data/media/movies", CollectionType: "movies"},
		}
	}
	return nil
}

// libraries returns the configured libraries, falling back to the defaults
// when Configure was not called (or was called with none).
func (c *Client) libraries() []Library {
	if c.configuredLibs != nil {
		return c.configuredLibs
	}
	return []Library{
		{Name: "TV Shows", Path: "/data/media/tv", CollectionType: "tvshows"},
		{Name: "Movies", Path: "/data/media/movies", CollectionType: "movies"},
	}
}

// wizardComplete reports whether the startup wizard has finished by reading
// StartupWizardCompleted from the public (anonymous) info endpoint.
func (c *Client) wizardComplete(ctx context.Context) (bool, error) {
	resp, err := c.httpGet(ctx, "/System/Info/Public")
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GET /System/Info/Public returned %s", resp.Status)
	}
	var info systemInfoPublic
	if err := json.NewDecoder(resp.Body).Decode(&info); err != nil {
		return false, fmt.Errorf("decode system info: %w", err)
	}
	return info.StartupWizardCompleted, nil
}

// startupUserPlaceholder reports whether the first user has not yet been
// created. Before creation, GET /Startup/User returns 200 {"Name":"root"}.
func (c *Client) startupUserPlaceholder(ctx context.Context) (bool, error) {
	resp, err := c.httpGet(ctx, "/Startup/User")
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		// Non-200 means auth is already required (wizard later stage): treat
		// as "no placeholder", i.e. skip user creation.
		return false, nil
	}
	var u startupUser
	if err := json.NewDecoder(resp.Body).Decode(&u); err != nil {
		return false, fmt.Errorf("decode startup user: %w", err)
	}
	return u.Name == "" || u.Name == "root", nil
}

// authenticate logs in as the admin user via the MediaBrowser scheme and
// returns the issued AccessToken.
func (c *Client) authenticate(ctx context.Context, username, password string) (string, error) {
	resp, err := c.httpPost(ctx, "/Users/AuthenticateByName", authenticateByName{Username: username, Pw: password}, c.mediaBrowserHeader())
	if err != nil {
		return "", err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return "", fmt.Errorf("POST /Users/AuthenticateByName returned %s: %s", resp.Status, strings.TrimSpace(string(body)))
	}
	var result authenticateResult
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return "", fmt.Errorf("decode authentication result: %w", err)
	}
	if result.AccessToken == "" {
		return "", fmt.Errorf("jellyfin: authentication returned no access token")
	}
	return result.AccessToken, nil
}

// hasAPIKey reports whether an API key named c.appName is already registered.
func (c *Client) hasAPIKey(ctx context.Context) (bool, error) {
	resp, err := c.httpGet(ctx, "/Auth/Keys?app="+url.QueryEscape(c.appName))
	if err != nil {
		return false, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return false, fmt.Errorf("GET /Auth/Keys returned %s", resp.Status)
	}
	var result authKeysResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return false, fmt.Errorf("decode api keys: %w", err)
	}
	for _, item := range result.Items {
		if item.AppName == c.appName {
			return true, nil
		}
	}
	return false, nil
}

// syncLibraries adds each configured library whose path is not already
// registered. The container must already expose each library path on disk
// (Jellyfin validates path existence).
func (c *Client) syncLibraries(ctx context.Context, libs []Library) error {
	existing, err := c.listLibraries(ctx)
	if err != nil {
		return err
	}
	havePath := map[string]bool{}
	for _, l := range existing {
		for _, p := range libraryPaths(l) {
			havePath[p] = true
		}
	}
	for _, lib := range libs {
		if havePath[lib.Path] {
			continue
		}
		query := url.Values{}
		query.Set("name", lib.Name)
		query.Set("collectionType", lib.CollectionType)
		body := libraryCreate{LibraryOptions: libraryOptions{
			PathInfos:                 []mediaPathInfo{{Path: lib.Path}},
			Enabled:                   true,
			EnableRealtimeMonitor:     false,
			PreferredMetadataLanguage: "en",
			MetadataCountryCode:       "US",
		}}
		if resp, err := c.httpPost(ctx, "/Library/VirtualFolders?"+query.Encode(), body, c.mediaBrowserHeader()); err != nil {
			return fmt.Errorf("jellyfin: add library %q: %w", lib.Name, err)
		} else {
			defer func() { _ = resp.Body.Close() }()
			switch resp.StatusCode {
			case http.StatusOK, http.StatusCreated, http.StatusNoContent:
			default:
				bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
				return fmt.Errorf("jellyfin: add library %q returned %s: %s", lib.Name, resp.Status, strings.TrimSpace(string(bodyBytes)))
			}
		}
	}
	return nil
}

// listLibraries returns the registered libraries (GET /Library/VirtualFolders).
func (c *Client) listLibraries(ctx context.Context) ([]libraryItem, error) {
	resp, err := c.httpGet(ctx, "/Library/VirtualFolders")
	if err != nil {
		return nil, err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET /Library/VirtualFolders returned %s", resp.Status)
	}
	var items []libraryItem
	if err := json.NewDecoder(resp.Body).Decode(&items); err != nil {
		return nil, fmt.Errorf("decode libraries: %w", err)
	}
	return items, nil
}

// libraryPaths returns the on-disk paths a registered library exposes,
// checking the LibraryOptions.PathInfos shape first and falling back to the
// top-level Locations shape used by some GET responses.
func libraryPaths(l libraryItem) []string {
	if len(l.LibraryOptions.PathInfos) > 0 {
		paths := make([]string, 0, len(l.LibraryOptions.PathInfos))
		for _, p := range l.LibraryOptions.PathInfos {
			paths = append(paths, p.Path)
		}
		return paths
	}
	paths := make([]string, 0, len(l.Locations))
	for _, p := range l.Locations {
		paths = append(paths, p.Path)
	}
	return paths
}

// mediaBrowserHeader builds the Jellyfin MediaBrowser authorization header,
// appending Token="..." once an access token has been issued.
func (c *Client) mediaBrowserHeader() http.Header {
	h := make(http.Header)
	h.Set("Authorization", fmt.Sprintf(
		`MediaBrowser Client="%s", Device="arrstart", DeviceId="%s", Version="%s"`,
		mediaBrowserClient, mediaBrowserDeviceID, mediaBrowserVersion))
	if c.token != "" {
		h.Get("Authorization")
		h.Set("Authorization", h.Get("Authorization")+`, Token="`+c.token+`"`)
	}
	return h
}

// httpGet issues an authenticated GET when a token is available.
func (c *Client) httpGet(ctx context.Context, path string) (*http.Response, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path, nil)
	if err != nil {
		return nil, err
	}
	for k, vs := range c.mediaBrowserHeader() {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return c.httpClient.Do(req)
}

// httpPost issues an authenticated POST with a JSON body. A nil body sends
// "{}" so Jellyfin's model binding receives an object.
func (c *Client) httpPost(ctx context.Context, path string, body any, extra http.Header) (*http.Response, error) {
	var r io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		r = strings.NewReader(string(b))
	} else {
		r = strings.NewReader("{}")
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, c.baseURL+path, r)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	for k, vs := range c.mediaBrowserHeader() {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	for k, vs := range extra {
		for _, v := range vs {
			req.Header.Add(k, v)
		}
	}
	return c.httpClient.Do(req)
}

// postJSON is a convenience for authenticated JSON POSTs expecting 200/204.
func (c *Client) postJSON(ctx context.Context, path string, body any, extra http.Header) error {
	resp, err := c.httpPost(ctx, path, body, extra)
	if err != nil {
		return err
	}
	defer func() { _ = resp.Body.Close() }()
	if resp.StatusCode != http.StatusOK && resp.StatusCode != http.StatusNoContent {
		bodyBytes, _ := io.ReadAll(io.LimitReader(resp.Body, 512))
		return fmt.Errorf("POST %s returned %s: %s", path, resp.Status, strings.TrimSpace(string(bodyBytes)))
	}
	return nil
}

// ---- request/response DTOs ----

type startupConfiguration struct {
	ServerName string `json:"ServerName"`
}

type startupUser struct {
	Name        string `json:"Name"`
	Password    string `json:"Password"`
	HasPassword bool   `json:"HasPassword"`
}

type authenticateByName struct {
	Username string `json:"Username"`
	Pw       string `json:"Pw"`
}

type authenticateResult struct {
	AccessToken string `json:"AccessToken"`
}

type authKeysResponse struct {
	Items []authKeyItem `json:"Items"`
}

type authKeyItem struct {
	AppName string `json:"AppName"`
}

type libraryCreate struct {
	LibraryOptions libraryOptions `json:"LibraryOptions"`
}

type libraryOptions struct {
	PathInfos                 []mediaPathInfo `json:"PathInfos"`
	Enabled                   bool            `json:"Enabled"`
	EnableRealtimeMonitor     bool            `json:"EnableRealtimeMonitor"`
	PreferredMetadataLanguage string          `json:"PreferredMetadataLanguage"`
	MetadataCountryCode       string          `json:"MetadataCountryCode"`
}

type mediaPathInfo struct {
	Path string `json:"Path"`
}

type libraryItem struct {
	Name           string          `json:"Name"`
	CollectionType string          `json:"CollectionType"`
	Locations      []mediaPathInfo `json:"Locations"` // alternate GET shape
	LibraryOptions libraryOptions  `json:"LibraryOptions"`
}

type systemInfoPublic struct {
	StartupWizardCompleted bool `json:"StartupWizardCompleted"`
}
