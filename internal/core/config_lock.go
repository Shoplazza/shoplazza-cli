package core

import (
	"path/filepath"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/lockfile"
)

// ConfigLockTimeout is the config.lock wait budget; on timeout callers
// fail loudly rather than hang.
const ConfigLockTimeout = 5 * time.Second

// LocksDir returns <config dir>/locks next to config.json.
func LocksDir(configPath string) string {
	return filepath.Join(filepath.Dir(configPath), "locks")
}

// UpdateConfig performs a locked read-modify-write on config.json.
// mutate returning an error aborts without writing.
func UpdateConfig(path string, timeout time.Duration, mutate func(*CliConfig) error) error {
	release, err := lockfile.Acquire(filepath.Join(LocksDir(path), "config.lock"), timeout)
	if err != nil {
		return err
	}
	defer release()
	cfg, err := LoadConfig(path)
	if err != nil {
		return err
	}
	if err := mutate(&cfg); err != nil {
		return err
	}
	return SaveConfig(path, cfg)
}
