package github

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
)

// cacheVersion is bumped when the Dashboard shape changes incompatibly; older files are ignored.
const cacheVersion = 2

type cacheFile struct {
	Version   int
	Dashboard *Dashboard
}

func cachePath(org string) (string, error) {
	dir := os.Getenv("GHX_CACHE_DIR")
	if dir == "" {
		base, err := os.UserCacheDir()
		if err != nil {
			return "", err
		}
		dir = filepath.Join(base, "ghx")
	}
	name := "dashboard.json"
	if org != "" {
		name = "dashboard-" + strings.ToLower(filepath.Base(org)) + ".json"
	}
	return filepath.Join(dir, name), nil
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
	b, err := json.Marshal(cacheFile{Version: cacheVersion, Dashboard: d})
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
	var f cacheFile
	if err := json.Unmarshal(b, &f); err != nil {
		return nil, err
	}
	if f.Version != cacheVersion || f.Dashboard == nil {
		return nil, os.ErrNotExist
	}
	return f.Dashboard, nil
}
