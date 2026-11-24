package credentials

import (
	"bufio"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"sort"
	"strings"
)

// LoadDotEnv reads path into a map. A missing file returns an empty map, nil
// error. Lines are "KEY=VALUE"; comments (#) and blank lines are ignored.
// Later duplicate keys overwrite earlier ones.
func LoadDotEnv(path string) (map[string]string, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return map[string]string{}, nil
		}
		return nil, fmt.Errorf("credentials: read .env: %w", err)
	}
	out := map[string]string{}
	scanner := bufio.NewScanner(strings.NewReader(string(data)))
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if line == "" || strings.HasPrefix(line, "#") {
			continue
		}
		k, v, found := strings.Cut(line, "=")
		if !found {
			continue
		}
		out[strings.TrimSpace(k)] = strings.TrimSpace(v)
	}
	if err := scanner.Err(); err != nil {
		return nil, fmt.Errorf("credentials: scan .env: %w", err)
	}
	return out, nil
}

// WriteDotEnv writes creds to path, deduping against existing entries.
// Idempotent: re-running with the same creds changes nothing. The file is
// created/written with 0o600 permissions because it contains secrets.
// Existing unrelated entries in .env are preserved.
func WriteDotEnv(path string, creds []Credential) error {
	existing, err := LoadDotEnv(path)
	if err != nil {
		return err
	}
	for _, c := range creds {
		existing[c.EnvName] = c.Key
	}
	keys := make([]string, 0, len(existing))
	for k := range existing {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	var sb strings.Builder
	for _, k := range keys {
		fmt.Fprintf(&sb, "%s=%s\n", k, existing[k])
	}
	return writeFileIfChanged(path, []byte(sb.String()), 0o600)
}

func writeFileIfChanged(path string, data []byte, perm os.FileMode) error {
	existing, err := os.ReadFile(path)
	if err == nil && bytesEqual(existing, data) {
		return nil
	}
	if err := os.WriteFile(path, data, perm); err != nil {
		return err
	}
	return nil
}

func bytesEqual(a, b []byte) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
