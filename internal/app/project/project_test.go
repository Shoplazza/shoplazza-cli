package project

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
)

func TestActiveConfig_DefaultsToShoplazzaAppToml(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "shoplazza.app.toml"), "client_id = \"cid_default\"\nscopes = [\"read\"]\n")

	p, err := Open(root)
	if err != nil {
		t.Fatal(err)
	}
	cfg, err := p.ActiveConfig()
	if err != nil {
		t.Fatal(err)
	}
	if cfg.ClientID != "cid_default" {
		t.Fatalf("client_id = %q", cfg.ClientID)
	}
}

func TestUseConfig_PersistsActivePointer(t *testing.T) {
	root := t.TempDir()
	writeFile(t, filepath.Join(root, "shoplazza.app.staging.toml"), "client_id = \"cid_staging\"\n")

	p, _ := Open(root)
	if err := p.SetActiveConfig("shoplazza.app.staging.toml", "cid_staging"); err != nil {
		t.Fatal(err)
	}
	p2, _ := Open(root)
	cfg, err := p2.ActiveConfig()
	if err != nil || cfg.ClientID != "cid_staging" {
		t.Fatalf("active = %+v, %v", cfg, err)
	}
	if _, err := os.Stat(filepath.Join(root, ".shoplazza", "app-state.json")); err != nil {
		t.Fatalf("state file missing: %v", err)
	}
}

func TestResolve(t *testing.T) {
	// Resolve returns OS-native paths, so build the expectations with filepath
	// rather than hard-coding forward slashes (which fail on Windows).
	cwd := filepath.FromSlash("/work")
	if got := Resolve(cwd, "."); got != cwd {
		t.Fatalf("Resolve(%q,.) = %q, want %q", cwd, got, cwd)
	}
	if got, want := Resolve(cwd, "sub"), filepath.Join(cwd, "sub"); got != want {
		t.Fatalf("Resolve(%q,sub) = %q, want %q", cwd, got, want)
	}
	abs := filepath.FromSlash("/abs")
	if runtime.GOOS == "windows" {
		abs = `C:\abs`
	}
	if got := Resolve(cwd, abs); got != abs {
		t.Fatalf("Resolve abs = %q, want %q", got, abs)
	}
}

func writeFile(t *testing.T, path, body string) {
	t.Helper()
	_ = os.MkdirAll(filepath.Dir(path), 0o755)
	if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestReadConfig_StringScopes(t *testing.T) {
	root := t.TempDir()
	// The canonical format — what the official app template ships (v1 parity).
	writeFile(t, filepath.Join(root, "shoplazza.app.toml"),
		"client_id = \"cid\"\nscopes = \"read_customer write_cart_transform\"\n")

	p, _ := Open(root)
	cfg, err := p.ReadConfig("shoplazza.app.toml")
	if err != nil {
		t.Fatalf("string-format scopes must read cleanly: %v", err)
	}
	if cfg.Scopes != "read_customer write_cart_transform" {
		t.Fatalf("scopes = %q, want the template string verbatim", cfg.Scopes)
	}
}

func TestReadConfig_LegacyArrayScopesStillReads(t *testing.T) {
	root := t.TempDir()
	// Files written by earlier v2 builds used a TOML array; read them leniently
	// by joining with spaces instead of erroring.
	writeFile(t, filepath.Join(root, "shoplazza.app.toml"),
		"client_id = \"cid\"\nscopes = [\"read_customer\", \"write_cart_transform\"]\n")

	p, _ := Open(root)
	cfg, err := p.ReadConfig("shoplazza.app.toml")
	if err != nil {
		t.Fatalf("legacy array-format scopes should read cleanly, got: %v", err)
	}
	if cfg.Scopes != "read_customer write_cart_transform" {
		t.Fatalf("scopes = %q, want space-joined string", cfg.Scopes)
	}
}

func TestUpdateConfig_PreservesExistingContent(t *testing.T) {
	root := t.TempDir()
	// Template file content must survive: scopes default and any extra keys.
	writeFile(t, filepath.Join(root, "shoplazza.app.toml"),
		"client_id= \"\"\nscopes = \"read_customer write_cart_transform\"\nembedded = true\n")

	p, _ := Open(root)
	if err := p.UpdateConfig("shoplazza.app.toml", map[string]any{
		"client_id":  "cid_new",
		"partner_id": "p9",
	}); err != nil {
		t.Fatalf("UpdateConfig: %v", err)
	}
	cfg, err := p.ReadConfig("shoplazza.app.toml")
	if err != nil {
		t.Fatalf("ReadConfig after update: %v", err)
	}
	if cfg.ClientID != "cid_new" || cfg.PartnerID != "p9" {
		t.Fatalf("updated fields not applied: %+v", cfg)
	}
	if cfg.Scopes != "read_customer write_cart_transform" {
		t.Fatalf("template scopes must be preserved, got %q", cfg.Scopes)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "shoplazza.app.toml"))
	if !strings.Contains(string(raw), "embedded = true") {
		t.Fatalf("unknown template keys must be preserved, file:\n%s", raw)
	}
}

func TestOpen_EmptyRoot_Errors(t *testing.T) {
	_, err := Open("")
	if err == nil {
		t.Error("expected error for empty root")
	}
}

func TestResolve_EmptyPath(t *testing.T) {
	// Empty path defaults to "."
	cwd := filepath.FromSlash("/work")
	if got := Resolve(cwd, ""); got != cwd {
		t.Errorf("Resolve with empty path = %q, want %q", got, cwd)
	}
}

func TestWriteConfig_RoundTrip(t *testing.T) {
	root := t.TempDir()
	p, _ := Open(root)

	cfg := Config{ClientID: "cid-wc", PartnerID: "p-wc", Scopes: "read_orders"}
	if err := p.WriteConfig("shoplazza.app.toml", cfg); err != nil {
		t.Fatalf("WriteConfig: %v", err)
	}
	got, err := p.ReadConfig("shoplazza.app.toml")
	if err != nil {
		t.Fatalf("ReadConfig after WriteConfig: %v", err)
	}
	if got.ClientID != cfg.ClientID || got.PartnerID != cfg.PartnerID {
		t.Errorf("round-trip mismatch: got %+v want %+v", got, cfg)
	}
}

// TestConfigName_TraversalRejected verifies --config style names must be bare
// file names — "../evil.toml" would otherwise escape the project root.
func TestConfigName_TraversalRejected(t *testing.T) {
	root := t.TempDir()
	outside := filepath.Join(t.TempDir(), "evil.toml")
	writeFile(t, outside, "client_id = \"cid_evil\"\n")
	p, _ := Open(root)

	rel, err := filepath.Rel(root, outside)
	if err != nil {
		t.Fatal(err)
	}
	for _, name := range []string{rel, "../evil.toml", "sub/evil.toml", "/etc/passwd", "", "."} {
		if _, rErr := p.ReadConfig(name); rErr == nil {
			t.Errorf("ReadConfig(%q) should be rejected", name)
		}
		if uErr := p.UpdateConfig(name, map[string]any{"client_id": "x"}); uErr == nil {
			t.Errorf("UpdateConfig(%q) should be rejected", name)
		}
		if wErr := p.WriteConfig(name, Config{ClientID: "x"}); wErr == nil {
			t.Errorf("WriteConfig(%q) should be rejected", name)
		}
		if sErr := p.SetActiveConfig(name, "x"); sErr == nil {
			t.Errorf("SetActiveConfig(%q) should be rejected", name)
		}
	}
	// Nothing outside the root may have been touched.
	data, err := os.ReadFile(outside)
	if err != nil || !strings.Contains(string(data), "cid_evil") {
		t.Fatalf("file outside the project root was modified: %q, %v", data, err)
	}
}

// TestWrites_AtomicShape verifies the crash-safety shape: config/state writes
// go through a temp-file+rename, so no stray *.tmp* files survive a successful
// write (and a crash mid-write can never truncate the live file in place).
func TestWrites_AtomicShape(t *testing.T) {
	root := t.TempDir()
	p, _ := Open(root)
	if err := p.WriteConfig("shoplazza.app.toml", Config{ClientID: "cid"}); err != nil {
		t.Fatal(err)
	}
	if err := p.UpdateConfig("shoplazza.app.toml", map[string]any{"partner_id": "p1"}); err != nil {
		t.Fatal(err)
	}
	if err := p.SetActiveConfig("shoplazza.app.toml", "cid"); err != nil {
		t.Fatal(err)
	}
	for _, dir := range []string{root, filepath.Join(root, ".shoplazza")} {
		entries, err := os.ReadDir(dir)
		if err != nil {
			t.Fatal(err)
		}
		for _, e := range entries {
			if strings.Contains(e.Name(), ".tmp") {
				t.Errorf("leftover temp file %s in %s", e.Name(), dir)
			}
		}
	}
	cfg, err := p.ActiveConfig()
	if err != nil || cfg.ClientID != "cid" || cfg.PartnerID != "p1" {
		t.Fatalf("round-trip after atomic writes: %+v, %v", cfg, err)
	}
}

func TestLoadState_CorruptFile_Errors(t *testing.T) {
	root := t.TempDir()
	// Write a corrupt JSON state file.
	stateFile := filepath.Join(root, ".shoplazza", "app-state.json")
	_ = os.MkdirAll(filepath.Dir(stateFile), 0o755)
	_ = os.WriteFile(stateFile, []byte("not valid json {{{"), 0o600)

	p, _ := Open(root)
	_, err := p.loadState()
	if err == nil {
		t.Error("expected error for corrupt state file")
	}
}

func TestActiveConfigName_MissingStateFile_DefaultsToToml(t *testing.T) {
	root := t.TempDir()
	p, _ := Open(root)
	name, err := p.ActiveConfigName()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != "shoplazza.app.toml" {
		t.Errorf("default config name = %q, want shoplazza.app.toml", name)
	}
}

func mustSeedKeychain(t *testing.T, account, value string) {
	t.Helper()
	if err := keychain.Set(keychain.ShoplazzaCliService, account, value); err != nil {
		t.Fatalf("mustSeedKeychain: %v", err)
	}
}

func getKeychain(t *testing.T, account string) string {
	t.Helper()
	v, err := keychain.Get(keychain.ShoplazzaCliService, account)
	if err != nil {
		t.Fatalf("getKeychain: %v", err)
	}
	return v
}
