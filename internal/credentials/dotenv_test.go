package credentials

import (
	"os"
	"path/filepath"
	"testing"
)

func TestLoadDotEnvMissingFile(t *testing.T) {
	got, err := LoadDotEnv(filepath.Join(t.TempDir(), "does-not-exist.env"))
	if err != nil {
		t.Fatalf("LoadDotEnv(missing): %v", err)
	}
	if len(got) != 0 {
		t.Errorf("LoadDotEnv(missing) = %v, want empty map", got)
	}
}

func TestLoadDotEnvParse(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte(`# comment line

KEY1=value1
  KEY2   =   value2
invalid-line-no-equals
KEY1=duplicate-wins
`), 0o600); err != nil {
		t.Fatalf("write .env: %v", err)
	}
	got, err := LoadDotEnv(path)
	if err != nil {
		t.Fatalf("LoadDotEnv: %v", err)
	}
	want := map[string]string{
		"KEY1": "duplicate-wins",
		"KEY2": "value2",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}

func TestWriteDotEnvCreates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	creds := []Credential{
		{App: "radarr", EnvName: "RADARR__AUTH__APIKEY", Key: "radarr-key"},
		{App: "sonarr", EnvName: "SONARR__AUTH__APIKEY", Key: "sonarr-key"},
	}
	if err := WriteDotEnv(path, creds); err != nil {
		t.Fatalf("WriteDotEnv: %v", err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read written .env: %v", err)
	}
	want := "RADARR__AUTH__APIKEY=radarr-key\nSONARR__AUTH__APIKEY=sonarr-key\n"
	if string(data) != want {
		t.Errorf("wrote %q, want %q", data, want)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatalf("stat .env: %v", err)
	}
	if got := info.Mode().Perm(); got != 0o600 {
		t.Errorf("perms = %o, want %o", got, 0o600)
	}
}

func TestWriteDotEnvDedupsAndPreserves(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, ".env")
	if err := os.WriteFile(path, []byte("# pre-existing\nUNRELATED=keep-me\nSONARR__AUTH__APIKEY=old-key\n"), 0o600); err != nil {
		t.Fatalf("seed .env: %v", err)
	}
	creds := []Credential{
		{App: "sonarr", EnvName: "SONARR__AUTH__APIKEY", Key: "new-key"},
	}
	if err := WriteDotEnv(path, creds); err != nil {
		t.Fatalf("WriteDotEnv: %v", err)
	}
	got, err := LoadDotEnv(path)
	if err != nil {
		t.Fatalf("LoadDotEnv after write: %v", err)
	}
	want := map[string]string{
		"UNRELATED":            "keep-me",
		"SONARR__AUTH__APIKEY": "new-key",
	}
	if len(got) != len(want) {
		t.Fatalf("got %d entries, want %d: %v", len(got), len(want), got)
	}
	for k, v := range want {
		if got[k] != v {
			t.Errorf("got[%q] = %q, want %q", k, got[k], v)
		}
	}
}
