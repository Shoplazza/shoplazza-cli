// Package fsx holds small filesystem helpers shared across the CLI.
package fsx

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

// ConfigDirEnv names the env var that relocates the CLI's login state:
// config.json (profiles, current store), auth.json, the keychain and the
// metadata cache. Point separate sessions at separate dirs to keep their
// logins and current store apart. Machine-wide state (update check, docs
// client id) stays in the OS config dir.
const ConfigDirEnv = "SHOPLAZZA_CLI_CONFIG_DIR"

// ConfigDir returns the directory holding the CLI's login state: the
// $SHOPLAZZA_CLI_CONFIG_DIR override when set, otherwise
// <os.UserConfigDir()>/shoplazza-cli. An invalid override is an error, never a
// fallback to the shared default — that would put this session's login where
// another session reads it.
func ConfigDir() (string, error) {
	dir, err := ConfigDirOverride()
	if err != nil || dir != "" {
		return dir, err
	}
	base, err := os.UserConfigDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(base, "shoplazza-cli"), nil
}

// ConfigDirOverride returns $SHOPLAZZA_CLI_CONFIG_DIR as a clean absolute path,
// or "" when it is unset or blank. A leading ~ expands to the home dir. Any
// other relative path is rejected: it would resolve against each command's
// working directory, so one session's state would split across dirs.
func ConfigDirOverride() (string, error) {
	dir := strings.TrimSpace(os.Getenv(ConfigDirEnv))
	if dir == "" {
		return "", nil
	}
	if dir == "~" || strings.HasPrefix(dir, "~/") || strings.HasPrefix(dir, "~"+string(filepath.Separator)) {
		home, err := os.UserHomeDir()
		if err != nil {
			return "", fmt.Errorf("%s: cannot expand ~: %w", ConfigDirEnv, err)
		}
		dir = filepath.Join(home, dir[1:])
	}
	if !filepath.IsAbs(dir) {
		return "", fmt.Errorf("%s must be an absolute path, got %q", ConfigDirEnv, dir)
	}
	return filepath.Clean(dir), nil
}

// WriteFileAtomic writes data to path via a temp file plus rename, so a crash
// mid-write never leaves a truncated file at path.
func WriteFileAtomic(path string, data []byte, perm os.FileMode) error {
	f, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".tmp*")
	if err != nil {
		return err
	}
	tmp := f.Name()
	if _, err := f.Write(data); err != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return err
	}
	if err := f.Close(); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	// CreateTemp opens 0600; apply the caller's perm before the file goes live.
	if err := os.Chmod(tmp, perm); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	if err := os.Rename(tmp, path); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return nil
}

// ReadJSON reads path and unmarshals it into a fresh T.
func ReadJSON[T any](path string) (*T, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var v T
	if err := json.Unmarshal(data, &v); err != nil {
		return nil, err
	}
	return &v, nil
}

// WriteJSON marshals v and writes it to path atomically, creating the parent
// directory if needed.
func WriteJSON(path string, v any, perm os.FileMode) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	return WriteFileAtomic(path, data, perm)
}
