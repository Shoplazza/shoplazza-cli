package theme_extension

import (
	"bytes"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	te "shoplazza-cli-v2/internal/theme_extension"
)

// writeCorruptConfig drops an undecodable extension.config.json into root.
func writeCorruptConfig(t *testing.T, root string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, "extension.config.json"),
		[]byte("{not valid json"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestBuild_RunE_CorruptConfigIsMalformedNotMissing: a present-but-undecodable
// config must surface the malformed message — NOT the "not a te project" /
// register-first guidance that would orphan the extension_id the file still
// holds.
func TestBuild_RunE_CorruptConfigIsMalformedNotMissing(t *testing.T) {
	root := t.TempDir()
	writeCorruptConfig(t, root)
	cmd := newCmdBuild(&cmdutil.Factory{})
	_ = cmd.Flags().Set("path", root)
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Fatal("expected error for corrupt config")
	}
	if !strings.Contains(err.Error(), "malformed") {
		t.Fatalf("expected the malformed message, got %q", err.Error())
	}
	if strings.Contains(err.Error(), "not a te project") {
		t.Fatalf("corrupt config must not be reported as missing: %q", err.Error())
	}
}

// TestDeploy_PreRunE_CorruptConfigIsMalformed: same contract on the
// RequireExtensionID path — no "register first" hint for a corrupt file.
func TestDeploy_PreRunE_CorruptConfigIsMalformed(t *testing.T) {
	root := t.TempDir()
	writeCorruptConfig(t, root)
	cmd := newCmdDeploy(&cmdutil.Factory{})
	_ = cmd.Flags().Set("version", "1.0.0")
	_ = cmd.Flags().Set("path", root)
	err := cmd.PreRunE(cmd, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(ee.Error(), "malformed") || strings.Contains(ee.Error(), "register first") {
		t.Fatalf("expected malformed message without the register hint, got %q", ee.Error())
	}
}

// newBuildStoreServer fakes the full store-side build chain: OSS sign, OSS
// POST, PUT theme-extensions, version-task create + poll.
func newBuildStoreServer(t *testing.T) *httptest.Server {
	t.Helper()
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/openapi/checkout_extensions/file/sign":
			_, _ = w.Write([]byte(`{"write_host":"` + srv.URL + `/oss","read_host":"` + srv.URL + `/read","policy":"p","access_id":"a","sign":"s"}`))
		case r.URL.Path == "/oss":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_e2e"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			_, _ = w.Write([]byte(`{"task_id":"t1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/t1":
			_, _ = w.Write([]byte(`{"task_id":"t1","state":1,"version_id":"ver_1"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	return srv
}

// TestBuild_EnvToken_EndToEnd_RemovesZip drives `te build` start to finish via
// the SHOPLAZZA_ACCESS_TOKEN bypass against a fake store and asserts the built
// zip is removed from .te-build/ after a successful upload (previously every
// build left one more zip behind, unbounded).
func TestBuild_EnvToken_EndToEnd_RemovesZip(t *testing.T) {
	srv := newBuildStoreServer(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL)

	root := t.TempDir()
	if err := te.WriteConfig(root, te.Config{Name: "ext-x", Type: "theme", Subtype: "basic"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "theme-app", "blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "theme-app", "blocks", "x.liquid"), []byte("<x>"), 0o644); err != nil {
		t.Fatal(err)
	}

	cmd := newCmdBuild(&cmdutil.Factory{})
	var out, errOut bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&errOut)
	cmd.SetArgs([]string{"--version", "1.0.0", "--description", "d", "--path", root, "--store-domain", "s.myshoplaza.com"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("build: %v (stderr: %s)", err, errOut.String())
	}
	if !strings.Contains(out.String(), "tex_e2e") {
		t.Fatalf("expected success envelope with extension_id, got %q", out.String())
	}
	zips, _ := filepath.Glob(filepath.Join(root, ".te-build", "*.zip"))
	if len(zips) != 0 {
		t.Fatalf("built zip must be removed after a successful upload, found %v", zips)
	}
}

// TestBuild_MissingThemeAppIsValidation / _UnreadableThemeAppIsInternal: only
// the absent/non-dir theme-app/ case is user-fixable validation; other zip
// failures are internal.
func TestBuild_MissingThemeAppIsValidation(t *testing.T) {
	srv := newBuildStoreServer(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL)

	root := t.TempDir() // config but NO theme-app/
	if err := te.WriteConfig(root, te.Config{Name: "ext-x", Type: "theme", Subtype: "basic"}); err != nil {
		t.Fatal(err)
	}
	cmd := newCmdBuild(&cmdutil.Factory{})
	cmd.SetArgs([]string{"--version", "1.0.0", "--description", "d", "--path", root, "--store-domain", "s.myshoplaza.com"})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected validation for missing theme-app/, got %v", err)
	}
}

func TestBuild_UnreadableThemeAppIsInternal(t *testing.T) {
	if os.Geteuid() == 0 {
		t.Skip("permission checks don't bind for root")
	}
	srv := newBuildStoreServer(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL)

	root := t.TempDir()
	if err := te.WriteConfig(root, te.Config{Name: "ext-x", Type: "theme", Subtype: "basic"}); err != nil {
		t.Fatal(err)
	}
	themeApp := filepath.Join(root, "theme-app")
	if err := os.MkdirAll(themeApp, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(themeApp, 0o000); err != nil { // exists, but unreadable → not a layout problem
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(themeApp, 0o755) })

	cmd := newCmdBuild(&cmdutil.Factory{})
	cmd.SetArgs([]string{"--version", "1.0.0", "--description", "d", "--path", root, "--store-domain", "s.myshoplaza.com"})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitInternal {
		t.Fatalf("expected internal for unreadable theme-app/, got %v", err)
	}
}

// TestBuild_UnparseableLatestSkipsGreaterCheck: a remote latest that isn't
// strict X.Y.Z makes the greater-than comparison inconclusive — the build must
// proceed instead of failing on garbage data.
func TestBuild_UnparseableLatestSkipsGreaterCheck(t *testing.T) {
	var srv *httptest.Server
	srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.URL.Path == "/openapi/2020-07/theme-extensions/tex_1/versions":
			_, _ = w.Write([]byte(`{"data":[{"version":"1.0.0-beta","version_id":"v9"}]}`))
		case r.URL.Path == "/openapi/checkout_extensions/file/sign":
			_, _ = w.Write([]byte(`{"write_host":"` + srv.URL + `/oss","read_host":"` + srv.URL + `/read","policy":"p","access_id":"a","sign":"s"}`))
		case r.URL.Path == "/oss":
			w.WriteHeader(http.StatusOK)
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			_, _ = w.Write([]byte(`{"task_id":"t1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/t1":
			_, _ = w.Write([]byte(`{"task_id":"t1","state":1,"version_id":"ver_2"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	t.Cleanup(srv.Close)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "tok_env")
	t.Setenv("SHOPLAZZA_CLI_API_BASE_URL", srv.URL)

	root := t.TempDir()
	if err := te.WriteConfig(root, te.Config{ExtensionID: "tex_1", Name: "ext-x", Type: "theme", Subtype: "basic"}); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(root, "theme-app", "blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "theme-app", "blocks", "x.liquid"), []byte("<x>"), 0o644); err != nil {
		t.Fatal(err)
	}
	cmd := newCmdBuild(&cmdutil.Factory{})
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&out)
	// 1.0.0 vs "1.0.0-beta": CompareVersions would call them equal (≤ → error);
	// the format gate must skip the check instead.
	cmd.SetArgs([]string{"--version", "1.0.0", "--description", "d", "--path", root, "--store-domain", "s.myshoplaza.com"})
	if err := cmd.Execute(); err != nil {
		t.Fatalf("build should skip the inconclusive version check, got %v", err)
	}
}
