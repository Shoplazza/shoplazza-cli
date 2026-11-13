package fsx_test

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/fsx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestWriteFileAtomic_CreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.json")
	if err := fsx.WriteFileAtomic(path, []byte(`{"a":1}`), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if string(got) != `{"a":1}` {
		t.Errorf("content = %q, want %q", got, `{"a":1}`)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	// Windows does not honor unix permission bits.
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o644 {
		t.Errorf("perm = %o, want 644", info.Mode().Perm())
	}
}

func TestWriteFileAtomic_ReplacesExisting(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "cfg.toml")
	if err := os.WriteFile(path, []byte("old content"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := fsx.WriteFileAtomic(path, []byte("new"), 0o644); err != nil {
		t.Fatalf("WriteFileAtomic over existing: %v", err)
	}
	got, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != "new" {
		t.Errorf("content = %q, want %q (must fully replace, not append)", got, "new")
	}
}

func TestWriteFileAtomic_AppliesPerm(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("unix permission bits not honored on windows")
	}
	dir := t.TempDir()
	path := filepath.Join(dir, "secret.env")
	if err := fsx.WriteFileAtomic(path, []byte("TOKEN=x"), 0o600); err != nil {
		t.Fatalf("WriteFileAtomic: %v", err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if info.Mode().Perm() != 0o600 {
		t.Errorf("perm = %o, want 600", info.Mode().Perm())
	}
}

// No temp file may survive a successful write — leftovers would accumulate in
// project directories and confuse globbing tools.
func TestWriteFileAtomic_NoTempLeftover(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "data.bin")
	if err := fsx.WriteFileAtomic(path, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatal(err)
	}
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Errorf("temp file left behind: %s", e.Name())
		}
	}
	if len(entries) != 1 {
		t.Errorf("dir has %d entries, want 1", len(entries))
	}
}

func TestWriteFileAtomic_MissingDir(t *testing.T) {
	path := filepath.Join(t.TempDir(), "no-such-dir", "out.txt")
	if err := fsx.WriteFileAtomic(path, []byte("x"), 0o644); err == nil {
		t.Error("expected error writing into a missing directory")
	}
}

func TestConfigDir_Override(t *testing.T) {
	testenv.IsolateConfigDir(t)
	home, err := os.UserHomeDir()
	if err != nil {
		t.Fatal(err)
	}
	abs := t.TempDir()
	for _, tc := range []struct{ name, env, want string }{
		{"absolute", abs, abs},
		{"surrounding whitespace trimmed", " " + abs + "\n", abs},
		{"cleaned", abs + string(filepath.Separator) + ".", abs},
		{"tilde expands to home", "~/state/dev", filepath.Join(home, "state", "dev")},
		{"bare tilde is home", "~", home},
	} {
		t.Run(tc.name, func(t *testing.T) {
			t.Setenv(fsx.ConfigDirEnv, tc.env)
			got, err := fsx.ConfigDir()
			if err != nil || got != tc.want {
				t.Fatalf("ConfigDir() = %q, %v; want %q", got, err, tc.want)
			}
		})
	}
}

// A relative override would resolve against each command's cwd, splitting one
// session's state across dirs; it must fail rather than fall back to the
// shared default dir.
func TestConfigDir_RelativeOverrideRejected(t *testing.T) {
	testenv.IsolateConfigDir(t)
	for _, env := range []string{"state/dev", "./state", "~other/state"} {
		t.Setenv(fsx.ConfigDirEnv, env)
		if got, err := fsx.ConfigDir(); err == nil || got != "" {
			t.Errorf("ConfigDir() with %q = %q, %v; want an error and no dir", env, got, err)
		}
	}
}

func TestConfigDir_BlankOverrideUsesDefault(t *testing.T) {
	testenv.IsolateConfigDir(t)
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	want := filepath.Join(base, "shoplazza-cli")
	for _, env := range []string{"", "  "} {
		t.Setenv(fsx.ConfigDirEnv, env)
		if got, err := fsx.ConfigDir(); err != nil || got != want {
			t.Errorf("ConfigDir() with %q = %q, %v; want %q", env, got, err, want)
		}
	}
}
