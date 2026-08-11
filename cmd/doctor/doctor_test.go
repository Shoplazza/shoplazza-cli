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

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
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

// newTestFactory builds a Factory pointed at an isolated, empty config dir. The
// same home redirect also hides the real ~/.agents/skills, so the skills check
// reports a machine-independent state.
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

// seedAuthLocksDirs creates the two directories checkAuthLocksDirs looks for.
func seedAuthLocksDirs(t *testing.T, configPath string) {
	t.Helper()
	if err := os.MkdirAll(internalauth.AuthDir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir auth: %v", err)
	}
	if err := os.MkdirAll(core.LocksDir(configPath), 0o700); err != nil {
		t.Fatalf("mkdir locks: %v", err)
	}
}

// GATE-11: a healthy v2 config (configVersion=2, auth/+locks/ present and
// writable) passes every check.
func TestDoctorCheck_V2Config_AllOK(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	seedAuthLocksDirs(t, configPath)

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
	meta := checks[2].(map[string]any)
	if meta["name"] != "metadata" || !strings.Contains(meta["message"].(string), "source=") {
		t.Errorf("metadata check malformed: %v", meta)
	}
	// The skills are opt-in, so an empty skills dir is still an ok verdict —
	// otherwise doctor would fail for everyone who never installed them.
	skills := checks[3].(map[string]any)
	if skills["name"] != "skills" || skills["message"] != "not installed" {
		t.Errorf("skills check malformed: %v", skills)
	}
}

// A skills directory with our skills in it flips the message, and stays ok.
func TestDoctorCheck_SkillsInstalled(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	seedAuthLocksDirs(t, configPath)

	skillsDir := testenv.SkillsDir(t)
	if err := os.MkdirAll(filepath.Join(skillsDir, "shoplazza-common"), 0o755); err != nil {
		t.Fatalf("mkdir skill: %v", err)
	}
	if err := os.WriteFile(filepath.Join(skillsDir, "shoplazza-common", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatalf("write skill: %v", err)
	}

	var got map[string]any
	if err := json.Unmarshal([]byte(runDoctorCmd(t, f, "check")), &got); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	checks, _ := got["checks"].([]any)
	skills := checks[3].(map[string]any)
	if skills["status"] != "ok" || skills["message"] != "installed" {
		t.Errorf("skills check should report installed and stay ok: %v", skills)
	}
}

// An unreadable skills dir silences every future refresh — it must not pass as
// an ok "not installed".
func TestDoctorCheck_SkillsUnreadable_Warns(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	seedAuthLocksDirs(t, configPath)

	skillsDir := testenv.SkillsDir(t)
	if err := os.Chmod(skillsDir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(skillsDir, 0o755) })

	var got map[string]any
	if err := json.Unmarshal([]byte(runDoctorCmd(t, f, "check")), &got); err != nil {
		t.Fatalf("output not JSON: %v", err)
	}
	if got["ok"] != false {
		t.Errorf("an unreadable skills dir must drag the overall verdict down: %v", got)
	}
	checks, _ := got["checks"].([]any)
	skills := checks[3].(map[string]any)
	if skills["status"] != "warn" || !strings.Contains(skills["message"].(string), "unreadable") {
		t.Errorf("skills check should warn: %v", skills)
	}
}

// GATE-12: a pre-v2 config warns that it has not migrated. The missing
// auth/locks directories are not part of that — they are created on first
// write, and the config directory here is writable, so nothing is broken.
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
	if byName["auth_locks_dirs"] != "ok" {
		t.Errorf("auth_locks_dirs = %q, want ok — lazily created dirs are not a finding", byName["auth_locks_dirs"])
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

// A leftover v1 auth.json next to a healthy v2 config is not a health
// problem, so it must not drag the verdict down — nothing about it stops a
// command from working.
func TestDoctorCheck_LeftoverV1AuthJSON_StaysOK(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg
	if err := os.WriteFile(filepath.Join(filepath.Dir(configPath), "auth.json"), []byte(`{}`), 0o600); err != nil {
		t.Fatalf("seed legacy auth.json: %v", err)
	}
	seedAuthLocksDirs(t, configPath)

	out := runDoctorCmd(t, f, "check")
	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["ok"] != true {
		t.Fatalf("expected ok=true with a leftover v1 auth.json, got %v", got)
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

// Neither directory exists yet and the config directory cannot be written, so
// the lazy creation will fail too. Absence alone is fine; absence with no way
// to fix it is the failure the check exists to name.
func TestDoctorCheck_ConfigDirNotWritable_Fails(t *testing.T) {
	f, configPath := newTestFactory(t)
	cfg := core.CliConfig{ConfigVersion: 2}
	if err := core.SaveConfig(configPath, cfg); err != nil {
		t.Fatalf("seed config: %v", err)
	}
	f.Config = cfg

	configDir := filepath.Dir(configPath)
	if err := os.Chmod(configDir, 0o500); err != nil {
		t.Fatalf("chmod config dir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chmod(configDir, 0o700) })
	skipIfDirWritable(t, configDir)

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
	if got["ok"] != false {
		t.Errorf("ok = %v, want false", got["ok"])
	}
}
