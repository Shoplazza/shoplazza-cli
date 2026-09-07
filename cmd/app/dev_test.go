package appcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/app/project"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestDev_Flags asserts the command registers the expected flags (no auth or
// tunnel work runs — the orchestration body is compile/vet-verified only).
func TestDev_Flags(t *testing.T) {
	cmd := newCmdDev(&cmdutil.Factory{})
	// --client-id / --partner were removed: dev now reads both from the active
	// config (partner is stored alongside client_id). --store-domain was removed
	// too: dev always targets the current store.
	for _, name := range []string{"path", "debug", "write-urls"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	if f := cmd.Flags().Lookup("write-urls"); f != nil && f.DefValue != "false" {
		t.Errorf("--write-urls must default to off, got %q", f.DefValue)
	}
	for _, name := range []string{"client-id", "partner", "store-domain"} {
		if cmd.Flags().Lookup(name) != nil {
			t.Errorf("flag --%s should have been removed (partner/client/store now come from config)", name)
		}
	}
	if cmd.PreRunE == nil {
		t.Error("expected PreRunE (requireLogin) to be set")
	}
}

// TestDev_RunE_NotAProjectErrors drives RunE to its first step (openProject) and
// asserts it errors when --path is not an app project. No auth/tunnel runs.
func TestDev_RunE_NotAProjectErrors(t *testing.T) {
	cmd := newCmdDev(&cmdutil.Factory{})
	if err := cmd.Flags().Set("path", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected an error when --path is not an app project")
	}
}

func TestLoadDotEnv(t *testing.T) {
	dir := t.TempDir()
	envPath := filepath.Join(dir, ".env")
	content := "" +
		"# a comment\n" +
		"\n" +
		"NGROK_AUTHTOKEN=tok123\n" +
		"NGROK_DOMAIN=\"my.domain\"\n" +
		"SINGLE='sq'\n" +
		"PRESET=should_not_override\n" +
		"noequalsline\n"
	if err := os.WriteFile(envPath, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	// A pre-existing env var must NOT be overridden.
	t.Setenv("PRESET", "original")
	// Ensure the parsed keys start unset so the test is deterministic.
	os.Unsetenv("NGROK_AUTHTOKEN")
	os.Unsetenv("NGROK_DOMAIN")
	os.Unsetenv("SINGLE")
	t.Cleanup(func() {
		os.Unsetenv("NGROK_AUTHTOKEN")
		os.Unsetenv("NGROK_DOMAIN")
		os.Unsetenv("SINGLE")
	})

	loadDotEnv(envPath)

	if got := os.Getenv("NGROK_AUTHTOKEN"); got != "tok123" {
		t.Errorf("NGROK_AUTHTOKEN = %q, want tok123", got)
	}
	if got := os.Getenv("NGROK_DOMAIN"); got != "my.domain" { // double quotes stripped
		t.Errorf("NGROK_DOMAIN = %q, want my.domain", got)
	}
	if got := os.Getenv("SINGLE"); got != "sq" { // single quotes stripped
		t.Errorf("SINGLE = %q, want sq", got)
	}
	if got := os.Getenv("PRESET"); got != "original" { // not overridden
		t.Errorf("PRESET = %q, want original (must not override existing env)", got)
	}

	// A missing file must be a no-op (no panic).
	loadDotEnv(filepath.Join(dir, "does-not-exist.env"))
}

// TestUpsertDotEnvReplacesAndPreserves: writing NGROK_AUTHTOKEN replaces the
// existing value in place and leaves every other line (keys, comments) intact.
func TestUpsertDotEnvReplacesAndPreserves(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := os.WriteFile(p, []byte("# my env\nFOO=bar\nNGROK_AUTHTOKEN=old\n"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := upsertDotEnv(p, "NGROK_AUTHTOKEN", "new_tok"); err != nil {
		t.Fatal(err)
	}
	s := readEnv(t, p)
	if !strings.Contains(s, "NGROK_AUTHTOKEN=new_tok") {
		t.Fatalf("value not replaced:\n%s", s)
	}
	if strings.Contains(s, "=old") {
		t.Fatalf("old value remained:\n%s", s)
	}
	if !strings.Contains(s, "FOO=bar") || !strings.Contains(s, "# my env") {
		t.Fatalf("unrelated lines lost:\n%s", s)
	}
}

// TestUpsertDotEnvCreatesAndAppends: a missing file is created (0600); a new key
// is appended without disturbing existing keys.
func TestUpsertDotEnvCreatesAndAppends(t *testing.T) {
	p := filepath.Join(t.TempDir(), ".env")
	if err := upsertDotEnv(p, "NGROK_AUTHTOKEN", "tok"); err != nil { // create
		t.Fatal(err)
	}
	if err := upsertDotEnv(p, "NGROK_DOMAIN", "x.ngrok.app"); err != nil { // append
		t.Fatal(err)
	}
	s := readEnv(t, p)
	if !strings.Contains(s, "NGROK_AUTHTOKEN=tok") || !strings.Contains(s, "NGROK_DOMAIN=x.ngrok.app") {
		t.Fatalf("create+append failed:\n%s", s)
	}
	// The .env holds secrets, so it must be written 0600 (POSIX file mode;
	// Windows has no equivalent permission bits).
	if runtime.GOOS != "windows" {
		fi, err := os.Stat(p)
		if err != nil {
			t.Fatal(err)
		}
		if fi.Mode().Perm() != 0o600 {
			t.Fatalf("secret file perms = %v, want 0600", fi.Mode().Perm())
		}
	}
}

func readEnv(t *testing.T, p string) string {
	t.Helper()
	b, err := os.ReadFile(p)
	if err != nil {
		t.Fatal(err)
	}
	return string(b)
}

// writeDevURLs creates the section, overwrites on rerun, keeps other keys.
func TestWriteDevURLs_CreatesMergesAndOverwrites(t *testing.T) {
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "shoplazza.app.toml"),
		[]byte("client_id = \"cid\"\nscopes = \"read\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	p, _ := project.Open(root)

	if err := writeDevURLs(p, "shoplazza.app.toml", "https://a.trycloudflare.com/auth", "https://a.trycloudflare.com/auth/callback"); err != nil {
		t.Fatalf("writeDevURLs: %v", err)
	}
	cfg, _ := p.ReadConfig("shoplazza.app.toml")
	if cfg.Dashboard.AppURL != "https://a.trycloudflare.com/auth" || cfg.Dashboard.RedirectURL != "https://a.trycloudflare.com/auth/callback" {
		t.Fatalf("dashboard after first write = %+v", cfg.Dashboard)
	}
	if cfg.ClientID != "cid" || cfg.Scopes != "read" {
		t.Fatalf("top-level keys must survive: %+v", cfg)
	}

	// User adds embed by hand, then a second dev session with a new tunnel.
	if err := p.UpdateConfig("shoplazza.app.toml", map[string]any{project.DashboardKey: map[string]any{"embed": true}}); err != nil {
		t.Fatal(err)
	}
	if err := writeDevURLs(p, "shoplazza.app.toml", "https://b.trycloudflare.com/auth", "https://b.trycloudflare.com/auth/callback"); err != nil {
		t.Fatal(err)
	}
	cfg, _ = p.ReadConfig("shoplazza.app.toml")
	if cfg.Dashboard.AppURL != "https://b.trycloudflare.com/auth" {
		t.Fatalf("second write must overwrite, got %q", cfg.Dashboard.AppURL)
	}
	if cfg.Dashboard.Embed == nil || !*cfg.Dashboard.Embed {
		t.Fatalf("embed must survive the URL rewrite, got %v", cfg.Dashboard.Embed)
	}
	raw, _ := os.ReadFile(filepath.Join(root, "shoplazza.app.toml"))
	if n := strings.Count(string(raw), project.DashboardComment); n != 1 {
		t.Fatalf("comment count = %d, want 1:\n%s", n, raw)
	}
	if n := strings.Count(string(raw), "app_url"); n != 1 {
		t.Fatalf("app_url must appear once, got %d:\n%s", n, raw)
	}
}

// writeDevURLs writes only the named config, leaving siblings alone.
func TestWriteDevURLs_TargetsNamedConfig(t *testing.T) {
	root := t.TempDir()
	_ = os.WriteFile(filepath.Join(root, "shoplazza.app.toml"), []byte("client_id = \"base\"\n"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "shoplazza.app.dev.toml"), []byte("client_id = \"dev\"\n"), 0o644)
	p, _ := project.Open(root)
	if err := writeDevURLs(p, "shoplazza.app.dev.toml", "https://t.dev/auth", "https://t.dev/auth/callback"); err != nil {
		t.Fatal(err)
	}
	base, _ := p.ReadConfig("shoplazza.app.toml")
	if base.Dashboard.AppURL != "" {
		t.Fatalf("base config must be untouched: %+v", base.Dashboard)
	}
	dev, _ := p.ReadConfig("shoplazza.app.dev.toml")
	if dev.Dashboard.AppURL != "https://t.dev/auth" {
		t.Fatalf("named config not written: %+v", dev.Dashboard)
	}
}

func TestDevNextSteps_TwoVariants(t *testing.T) {
	res := app.DevResult{InstallURL: "https://install", AppURL: "https://t/auth", RedirectURL: "https://t/auth/callback"}
	// Without --write-urls the text must not claim anything was written.
	plain := devNextSteps(res, "/proj", "")
	for _, want := range []string{"--write-urls", "cd /proj && shoplazza app config push", "https://t/auth", "https://t/auth/callback", "https://install"} {
		if !strings.Contains(plain, want) {
			t.Errorf("plain next steps missing %q:\n%s", want, plain)
		}
	}
	if strings.Contains(plain, "written to") {
		t.Errorf("plain next steps must not claim URLs were written:\n%s", plain)
	}
	written := devNextSteps(res, "/proj", "shoplazza.app.toml")
	for _, want := range []string{"written to shoplazza.app.toml", "cd /proj && shoplazza app config push", "https://install"} {
		if !strings.Contains(written, want) {
			t.Errorf("write-urls next steps missing %q:\n%s", want, written)
		}
	}
	if strings.Contains(written, "Redirect URL:") {
		t.Errorf("write-urls variant should not repeat the URLs:\n%s", written)
	}
}
