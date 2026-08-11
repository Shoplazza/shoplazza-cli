// Package testenv holds shared helpers for isolating per-test process state.
package testenv

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillsync"
)

// configDirEnv is every env var os.UserConfigDir() / os.UserHomeDir() consult,
// pointed at dir. On Unix both are derived from HOME / XDG_CONFIG_HOME; on
// Windows os.UserConfigDir() reads %AppData% and os.UserHomeDir() reads
// %USERPROFILE% (both ignore HOME/XDG), so all must be redirected — otherwise
// keychain/auth tests hit the real user config dir (or error with "%AppData%
// is not defined").
func configDirEnv(dir string) [][2]string {
	return [][2]string{
		{"HOME", dir},
		{"USERPROFILE", dir},
		{"XDG_CONFIG_HOME", filepath.Join(dir, ".config")},
		{"AppData", filepath.Join(dir, "AppData", "Roaming")},
		{"LOCALAPPDATA", filepath.Join(dir, "AppData", "Local")},
	}
}

// IsolateConfigDir points os.UserConfigDir() / os.UserHomeDir() at a fresh temp
// directory for the duration of t, and returns its root.
func IsolateConfigDir(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	for _, kv := range configDirEnv(dir) {
		t.Setenv(kv[0], kv[1])
	}
	return dir
}

// SkillsDir creates and returns the Agent Skills directory for the current home.
// Isolate the home first, or this touches the developer's own ~/.agents/skills.
func SkillsDir(t *testing.T) string {
	t.Helper()
	dir := skillsync.Dir()
	if dir == "" {
		t.Fatal("skills dir unresolved: isolate the home directory first")
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatalf("mkdir skills dir: %v", err)
	}
	return dir
}

// IsolateSkillsDir redirects the home to a temp dir and returns the empty skills
// directory inside it — the only relocation `npx skills add -g` also follows.
func IsolateSkillsDir(t *testing.T) string {
	t.Helper()
	IsolateConfigDir(t)
	return SkillsDir(t)
}

// RunMainIsolated is the whole body of a TestMain that needs an isolated config
// dir for the entire binary — packages whose tests call registry.LoadSpec, so
// they compare against the embedded spec and never a real user's downloaded
// metadata cache. It redirects, runs m, cleans up and exits.
func RunMainIsolated(m *testing.M) {
	dir, err := os.MkdirTemp("", "isolated-config-*")
	if err != nil {
		os.Exit(1)
	}
	for _, kv := range configDirEnv(dir) {
		os.Setenv(kv[0], kv[1])
	}
	code := m.Run()
	os.RemoveAll(dir)
	os.Exit(code)
}
