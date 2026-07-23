package core

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestDefaultConfigPath_NonEmpty(t *testing.T) {
	got, err := DefaultConfigPath()
	if err != nil {
		t.Fatalf("DefaultConfigPath: %v", err)
	}
	if got == "" {
		t.Error("DefaultConfigPath returned empty string")
	}
}

func TestLoadConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	cfg, err := LoadConfig(filepath.Join(dir, "no_such.json"))
	if err != nil {
		t.Fatalf("missing file should return empty config, got: %v", err)
	}
	if cfg.CurrentProfile != "" || len(cfg.Profiles) != 0 {
		t.Errorf("missing file: got non-empty config %+v", cfg)
	}
}

func TestSaveAndLoadConfig_RoundTrip(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	want := CliConfig{CurrentProfile: "us", Profiles: []ProfileConfig{{Name: "us", StoreDomain: "shop.myshoplaza.com"}}}
	if err := SaveConfig(path, want); err != nil {
		t.Fatalf("SaveConfig: %v", err)
	}
	got, err := LoadConfig(path)
	if err != nil {
		t.Fatalf("LoadConfig: %v", err)
	}
	if got.CurrentProfile != want.CurrentProfile || len(got.Profiles) != 1 || got.Profiles[0].StoreDomain != want.Profiles[0].StoreDomain {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, want)
	}
}

func TestSaveConfig_CreatesParentDir(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "sub", "dir", "config.json")
	if err := SaveConfig(path, CliConfig{CurrentProfile: "x"}); err != nil {
		t.Fatalf("SaveConfig with nested path: %v", err)
	}
	if _, err := os.Stat(path); err != nil {
		t.Errorf("config file not created: %v", err)
	}
}

func TestRemoveConfig_ExistingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	_ = SaveConfig(path, CliConfig{CurrentProfile: "x"})
	if err := RemoveConfig(path); err != nil {
		t.Fatalf("RemoveConfig: %v", err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Error("file should be deleted")
	}
}

func TestRemoveConfig_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "no_such.json")
	if err := RemoveConfig(path); err != nil {
		t.Fatalf("RemoveConfig on missing file should not error: %v", err)
	}
}

func TestLoadConfig_InvalidJSON(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "config.json")
	if err := os.WriteFile(path, []byte("not json"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := LoadConfig(path)
	if err == nil {
		t.Error("expected error for invalid JSON")
	}
}

func TestLoadConfig_EmptyPath_UsesDefaultPath(t *testing.T) {
	testenv.IsolateConfigDir(t)
	// An empty path triggers the DefaultConfigPath() fallback.
	cfg, err := LoadConfig("")
	if err != nil {
		t.Skipf("DefaultConfigPath unavailable: %v", err)
	}
	_ = cfg
}

func TestSaveConfig_EmptyPath_UsesDefaultPath(t *testing.T) {
	testenv.IsolateConfigDir(t)
	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Skip("DefaultConfigPath unavailable")
	}
	if err := SaveConfig("", CliConfig{CurrentProfile: "test-acct"}); err != nil {
		t.Fatalf("SaveConfig with empty path: %v", err)
	}
	loaded, err := LoadConfig(defaultPath)
	if err != nil {
		t.Fatalf("LoadConfig after save: %v", err)
	}
	if loaded.CurrentProfile != "test-acct" {
		t.Errorf("got %q want test-acct", loaded.CurrentProfile)
	}
}

func TestRemoveConfig_EmptyPath(t *testing.T) {
	testenv.IsolateConfigDir(t)
	defaultPath, err := DefaultConfigPath()
	if err != nil {
		t.Skip("DefaultConfigPath unavailable")
	}
	// Remove first, then remove again via empty path to confirm idempotency.
	_ = os.Remove(defaultPath)
	if err := RemoveConfig(""); err != nil {
		t.Fatalf("RemoveConfig with empty path on missing file: %v", err)
	}
}

func TestValidateProfileName(t *testing.T) {
	valid := []string{"us", "cn-staging", "a.b_c-1", strings.Repeat("a", 64)}
	invalid := []string{"", ".hidden", "-flag", "_x", strings.Repeat("a", 65),
		"has space", "汉字", "a:b", "con", "NUL", "com3", "Lpt9"}
	for _, n := range valid {
		if err := ValidateProfileName(n); err != nil {
			t.Errorf("%q should be valid: %v", n, err)
		}
	}
	for _, n := range invalid {
		if err := ValidateProfileName(n); err == nil {
			t.Errorf("%q should be rejected", n)
		}
	}
}

func TestDeriveProfileName(t *testing.T) {
	taken := map[string]bool{"us": true}
	isTaken := func(n string) bool { return taken[strings.ToLower(n)] }
	if got := DeriveProfileName("cn.myshoplazza.com", isTaken); got != "cn" {
		t.Fatalf("got %q", got)
	}
	if got := DeriveProfileName("us.myshoplazza.com", isTaken); got != "us-2" {
		t.Fatalf("conflict suffix: got %q", got)
	}
	if got := DeriveProfileName("shop.example.com", isTaken); got != "shop.example.com" {
		t.Fatalf("custom domain keeps full host: got %q", got)
	}
	// Real platform domains are .myshoplaza.com (single z), with optional env
	// segments (stg/dev): the default name is always the first label.
	if got := DeriveProfileName("abctt.myshoplaza.com", isTaken); got != "abctt" {
		t.Fatalf("single-z platform domain: got %q, want abctt", got)
	}
	if got := DeriveProfileName("neymar.stg.myshoplaza.com", isTaken); got != "neymar" {
		t.Fatalf("env-segmented domain: got %q, want neymar", got)
	}
	if got := DeriveProfileName("xjn.dev.myshoplaza.com", isTaken); got != "xjn" {
		t.Fatalf("dev env domain: got %q, want xjn", got)
	}
}

// A store subdomain that is a Windows-reserved device name (e.g. "con") must not
// derive to that bare name — the auto-created profile's meta file (con.json) is
// unusable on Windows. The derived name must always pass ValidateProfileName.
func TestDeriveProfileName_ReservedNameStaysValid(t *testing.T) {
	notTaken := func(string) bool { return false }
	got := DeriveProfileName("con.myshoplazza.com", notTaken)
	if got == "con" {
		t.Fatalf("derived name must not be the bare reserved word %q", got)
	}
	if err := ValidateProfileName(got); err != nil {
		t.Fatalf("derived name %q must be a valid profile name, got: %v", got, err)
	}
}

func TestFindProfile_CaseInsensitive(t *testing.T) {
	c := CliConfig{Profiles: []ProfileConfig{{Name: "prod-us", StoreDomain: "us.myshoplazza.com"}}}
	if c.FindProfile("Prod-US") == nil || c.FindProfileByStore("US.myshoplazza.com") == nil {
		t.Fatal("lookup must be case-insensitive")
	}
}

func TestCurrentStoreDomain_NoProfile(t *testing.T) {
	c := CliConfig{}
	if c.CurrentStoreDomain() != "" {
		t.Fatal("no current profile → empty")
	}
}

func TestCurrentStoreDomain_UsesCurrentProfile(t *testing.T) {
	c := CliConfig{CurrentProfile: "us",
		Profiles: []ProfileConfig{{Name: "us", StoreDomain: "us.myshoplazza.com"}}}
	if c.CurrentStoreDomain() != "us.myshoplazza.com" {
		t.Fatal("must return current profile's store domain")
	}
}

func TestSaveConfig_Atomic(t *testing.T) {
	p := filepath.Join(t.TempDir(), "config.json")
	if err := SaveConfig(p, CliConfig{ConfigVersion: 2}); err != nil {
		t.Fatal(err)
	}
	// 目录里不残留 .tmp
	entries, _ := os.ReadDir(filepath.Dir(p))
	for _, e := range entries {
		if strings.Contains(e.Name(), ".tmp") {
			t.Fatalf("tmp leak: %s", e.Name())
		}
	}
}
