package doctor

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"strings"
	"testing"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/testenv"
)

// runDoctorCmd runs the doctor command tree with args, capturing stdout, and
// fails the test on any RunE error.
func runDoctorCmd(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := NewCmdDoctor(f)
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("doctor %v: unexpected error: %v", args, err)
	}
	return buf.String()
}

// newTestFactory builds a Factory pointed at an isolated, empty config dir.
func newTestFactory(t *testing.T) (*cmdutil.Factory, string) {
	t.Helper()
	dir := testenv.IsolateConfigDir(t)
	configPath := filepath.Join(dir, "config.json")
	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{Out: io.Discard, ErrOut: io.Discard},
		ConfigPath: configPath,
	}, configPath
}

func TestNewCmdDoctor_Structure(t *testing.T) {
	f, _ := newTestFactory(t)
	cmd := NewCmdDoctor(f)
	if cmd.Use != "doctor" {
		t.Errorf("Use = %q, want doctor", cmd.Use)
	}
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "check" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'check' subcommand under doctor")
	}
}

// GATE-11: a healthy v2 config (configVersion=2, auth/+locks/ present and
// writable, no leftover v1 auth.json) passes every check.
func TestDoctorCheck_V2Config_AllOK(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	if err := os.MkdirAll(internalauth.AuthDir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.MkdirAll(core.LocksDir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir locks: %v", err)
	}

	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["ok"] != true {
		t.Fatalf("expected ok=true for a healthy v2 config, got %v", got)
	}
	checks, _ := got["checks"].([]any)
	if len(checks) != 4 {
		t.Fatalf("expected 4 checks, got %d: %v", len(checks), checks)
	}
	for _, c := range checks {
		m := c.(map[string]any)
		if m["status"] != "ok" {
			t.Errorf("check %v not ok: %v", m["name"], m)
		}
	}
	meta := checks[3].(map[string]any)
	if meta["name"] != "metadata" || !strings.Contains(meta["message"].(string), "source=") {
		t.Errorf("metadata check malformed: %v", meta)
	}
}

// GATE-12: a pre-v2 config with no auth/locks directories yet warns on both
// the configVersion and migration-residue checks, and the directory check
// warns rather than fails (directories are created lazily).
func TestDoctorCheck_V1MissingDirs_Warns(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{} // unmigrated: ConfigVersion 0, no profiles
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	// No auth/ or locks/ dirs created — simulates a config that predates any
	// v2 write.

	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["ok"] != false {
		t.Fatalf("expected ok=false for an unmigrated config, got %v", got)
	}
	byName := map[string]string{}
	for _, c := range got["checks"].([]any) {
		m := c.(map[string]any)
		byName[m["name"].(string)] = m["status"].(string)
	}
	if byName["config_version"] != "warn" {
		t.Errorf("config_version = %q, want warn", byName["config_version"])
	}
	if byName["auth_locks_dirs"] != "warn" {
		t.Errorf("auth_locks_dirs = %q, want warn", byName["auth_locks_dirs"])
	}
	if byName["migration_residue"] != "warn" {
		t.Errorf("migration_residue = %q, want warn", byName["migration_residue"])
	}
}

// A fresh install (no config.json at all) is healthy, not a warning — there
// is nothing to migrate yet.
func TestDoctorCheck_FreshInstall_AllOK(t *testing.T) {
	f, _ := newTestFactory(t)
	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["ok"] != true {
		t.Fatalf("expected ok=true on a fresh install, got %v", got)
	}
}

// A v2 config with a leftover v1 auth.json alongside it is migration
// residue: warn, even though configVersion itself is fine.
func TestDoctorCheck_LeftoverV1AuthJSON_Warns(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed legacy auth.json: %v", err)
	}

	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["ok"] != false {
		t.Fatalf("expected ok=false with leftover v1 auth.json, got %v", got)
	}
}

// skipIfDirWritable skips when a write into dir still succeeds despite a
// prior chmod 0o555 (root, or a filesystem that ignores permissions) — the
// fail-path can't be induced there, so skipping avoids a false negative.
func skipIfDirWritable(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, ".write-probe")
	if wf, err := os.Create(probe); err == nil {
		_ = wf.Close()
		_ = os.Remove(probe)
		t.Skipf("%s is writable despite chmod 0o555; cannot exercise the write-failure path", dir)
	}
}

// A locks/ directory that exists but isn't writable fails, not warns —
// commands that update config.json would hang/error.
func TestDoctorCheck_LocksNotWritable_Fails(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	if err := os.MkdirAll(internalauth.AuthDir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	locksDir := core.LocksDir(configPath)
	if err := os.MkdirAll(locksDir, 0o500); err != nil {
		t.Fatalf("mkdir locks: %v", err)
	}
	skipIfDirWritable(t, locksDir)

	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	byName := map[string]string{}
	for _, c := range got["checks"].([]any) {
		m := c.(map[string]any)
		byName[m["name"].(string)] = m["status"].(string)
	}
	if byName["auth_locks_dirs"] != "fail" {
		t.Errorf("auth_locks_dirs = %q, want fail", byName["auth_locks_dirs"])
	}
}
