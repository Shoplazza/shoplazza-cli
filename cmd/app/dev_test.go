package appcmd

import (
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestDev_Flags asserts the command registers the expected flags (no auth or
// tunnel work runs — the orchestration body is compile/vet-verified only).
func TestDev_Flags(t *testing.T) {
	cmd := newCmdDev(&cmdutil.Factory{})
	// --client-id / --partner were removed: dev now reads both from the active
	// config (partner is stored alongside client_id). --store-domain was removed
	// too: dev always targets the current store.
	for _, name := range []string{"path", "debug"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
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
