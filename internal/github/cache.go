package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

func cachePath(org string) (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	name := "dashboard.json"
	if org != "" {
		name = "dashboard-" + strings.ToLower(filepath.Base(org)) + ".json"
	}
	return filepath.Join(dir, "ghx", name), nil
}

// SaveCache writes the last live dashboard so the next launch can paint instantly.
// The file is user-only (0600) because it may contain private repository names.
func SaveCache(d *Dashboard, org string) error {
	if d == nil || d.Demo {
		return nil
	}
	path, err := cachePath(org)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	b, err := json.Marshal(d)
	if err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, b, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// LoadCache returns the last saved live dashboard, if any.
func LoadCache(org string) (*Dashboard, error) {
	path, err := cachePath(org)
	if err != nil {
		return nil, err
	}
	b, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var d Dashboard
	if err := json.Unmarshal(b, &d); err != nil {
		return nil, err
	}
	return &d, nil
}
