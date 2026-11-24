package credentials

import (
	"strings"
	"testing"
	"unicode"
)

func TestDefaults(t *testing.T) {
	if len(Defaults) == 0 {
		t.Fatal("Defaults is empty")
	}
	expected := map[string]string{
		"sonarr":   "SONARR__AUTH__APIKEY",
		"radarr":   "RADARR__AUTH__APIKEY",
		"prowlarr": "PROWLARR__AUTH__APIKEY",
	}
	got := map[string]string{}
	for _, s := range Defaults {
		got[s.App] = s.EnvName
	}
	for app, env := range expected {
		if got[app] != env {
			t.Errorf("Defaults[%q] = %q, want %q", app, got[app], env)
		}
	}
}

func TestGenerateDefaultKeys(t *testing.T) {
	creds, err := GenerateDefaultKeys()
	if err != nil {
		t.Fatalf("GenerateDefaultKeys: %v", err)
	}
	if len(creds) != len(Defaults) {
		t.Fatalf("got %d creds, want %d", len(creds), len(Defaults))
	}
	envs := map[string]bool{}
	for _, c := range creds {
		if c.Key == "" {
			t.Errorf("credential for %s has empty key", c.App)
		}
		if c.EnvName == "" {
			t.Errorf("credential for %s has empty env name", c.App)
		}
		if len(c.Key) != 64 {
			t.Errorf("credential for %s key length = %d, want 64", c.App, len(c.Key))
		}
		for _, r := range c.Key {
			if !unicode.Is(unicode.Hex_Digit, r) {
				t.Errorf("credential for %s key contains non-hex char %q", c.App, r)
			}
		}
		envs[c.EnvName] = true
	}
	for _, s := range Defaults {
		if !envs[s.EnvName] {
			t.Errorf("missing credential for env %s", s.EnvName)
		}
	}
}

func TestGenerateKeysAreUnique(t *testing.T) {
	a, err := GenerateDefaultKeys()
	if err != nil {
		t.Fatalf("first GenerateDefaultKeys: %v", err)
	}
	b, err := GenerateDefaultKeys()
	if err != nil {
		t.Fatalf("second GenerateDefaultKeys: %v", err)
	}
	same := 0
	for _, ca := range a {
		for _, cb := range b {
			if ca.App == cb.App && ca.Key == cb.Key {
				same++
			}
		}
	}
	if same != 0 {
		t.Fatalf("expected distinct keys across generations, got %d duplicates", same)
	}
}

// TestGenerateWithEmptySpecs is a sanity check: no specs means no creds, no error.
func TestGenerateWithEmptySpecs(t *testing.T) {
	creds, err := Generate(nil)
	if err != nil {
		t.Fatalf("Generate(nil): %v", err)
	}
	if len(creds) != 0 {
		t.Fatalf("got %d creds, want 0", len(creds))
	}
}

func TestGenerateKeyFormat(t *testing.T) {
	key, err := GenerateKey()
	if err != nil {
		t.Fatalf("GenerateKey: %v", err)
	}
	if len(key) != 64 {
		t.Fatalf("GenerateKey length = %d, want 64", len(key))
	}
	if strings.ContainsAny(key, "ghijklmnopqrstuvwxyzGHIOPQRSTUVWXYZ") {
		t.Errorf("GenerateKey contains non-hex chars: %q", key)
	}
}
