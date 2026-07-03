// Package testenv holds shared helpers for isolating per-test process state.
package testenv

import (
	"path/filepath"
	"testing"
)

// IsolateConfigDir points os.UserConfigDir() / os.UserHomeDir() at a fresh temp
// directory on every platform and returns its root. On Unix both are derived
// from HOME / XDG_CONFIG_HOME; on Windows os.UserConfigDir() reads %AppData%
// and os.UserHomeDir() reads %USERPROFILE% (both ignore HOME/XDG), so all must
// be redirected — otherwise keychain/auth tests hit the real user config dir
// (or error with "%AppData% is not defined").
func IsolateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("USERPROFILE", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("AppData", filepath.Join(dir, "AppData", "Roaming"))
	t.Setenv("LOCALAPPDATA", filepath.Join(dir, "AppData", "Local"))
	return dir
}
