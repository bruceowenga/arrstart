// Package credentials generates and stores the pre-generated API keys used to
// authenticate the wired apps (PRD A1, A9).
//
// Servarr apps accept their key via environment variables at container
// creation (SONARR__AUTH__APIKEY, RADARR__AUTH__APIKEY,
// PROWLARR__AUTH__APIKEY — verified live against the pinned images in
// docs/spikes/01-servarr-api-keys.md). This package generates those keys,
// tracks their env var names, and persists them to .env with secure
// permissions so they are available to both compose injection and the wiring
// clients.
//
// A fallback path (reading config.xml from the app's config volume, per A1)
// is not part of this package; it is handled per-app by the wiring clients
// when env injection is unavailable.
package credentials

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
)

// AppSpec describes one app's credential requirements.
type AppSpec struct {
	App     string // app name, e.g. "sonarr"
	EnvName string // env var used to inject the key, e.g. "SONARR__AUTH__APIKEY"
}

// Defaults lists the Servarr apps WP3.2 handles, with their injection env var
// names. Verified live against pinned Sonarr/Radarr/Prowlarr containers in
// docs/spikes/01-servarr-api-keys.md (spike S1).
var Defaults = []AppSpec{
	{App: "sonarr", EnvName: "SONARR__AUTH__APIKEY"},
	{App: "radarr", EnvName: "RADARR__AUTH__APIKEY"},
	{App: "prowlarr", EnvName: "PROWLARR__AUTH__APIKEY"},
}

// Credential is a generated secret ready to inject or store.
type Credential struct {
	App     string
	EnvName string
	Key     string
}

// Generate returns Credentials for each spec, each with a fresh random key.
func Generate(specs []AppSpec) ([]Credential, error) {
	creds := make([]Credential, 0, len(specs))
	for _, spec := range specs {
		key, err := GenerateKey()
		if err != nil {
			return nil, fmt.Errorf("credentials: generate key for %s: %w", spec.App, err)
		}
		creds = append(creds, Credential{
			App:     spec.App,
			EnvName: spec.EnvName,
			Key:     key,
		})
	}
	return creds, nil
}

// GenerateDefaultKeys returns credentials for all Defaults apps.
func GenerateDefaultKeys() ([]Credential, error) {
	return Generate(Defaults)
}

// GenerateKey returns a random 64-character hex key (32 bytes).
func GenerateKey() (string, error) {
	b := make([]byte, 32)
	if _, err := rand.Read(b); err != nil {
		return "", err
	}
	return hex.EncodeToString(b), nil
}
