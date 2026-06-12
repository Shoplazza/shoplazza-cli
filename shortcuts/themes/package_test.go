package themes

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// flagsWithNoIgnore builds a FlagSet over a freshly-constructed cobra command
// so that GetBool("no-ignore") returns the supplied value. Mirrors the
// flagsWithName helper used by init_test.go for symmetry.
func flagsWithNoIgnore(noIgnore bool) common.FlagSet {
	cmd := &cobra.Command{Use: "package"}
	cmd.Flags().Bool("no-ignore", noIgnore, "")
	return common.NewCobraFlagSet(cmd)
}

// makeThemeAt populates dir with a minimal but valid theme directory
// structure: assets/main.css, layout/theme.liquid. Used by the .themeignore
// scenarios and the basic packing tests. Mirrors the helper shape called out
// in the plan.
func makeThemeAt(t *testing.T, dir string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "assets", "main.css"), []byte("/* css */"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "layout"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "layout", "theme.liquid"), []byte("<html>"), 0o644); err != nil {
		t.Fatal(err)
	}
}

// writeSettings writes config/settings_schema.json under dir with the given
// theme_name / theme_version. Pass "" for either field to omit it (so we can
// exercise the fallback paths).
func writeSettings(t *testing.T, dir, name, version string) {
	t.Helper()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	var inner string
	switch {
	case name != "" && version != "":
		inner = `,"theme_name":"` + name + `","theme_version":"` + version + `"`
	case name == "" && version != "":
		inner = `,"theme_version":"` + version + `"`
	case name != "" && version == "":
		inner = `,"theme_name":"` + name + `"`
	default:
		inner = ""
	}
	// Real settings_schema.json is a JSON ARRAY; theme_info is the element with
	// name=="theme_info". A second section uses a localized-object name to lock
	// in that readThemeInfo tolerates mixed name types.
	content := `[{"name":"theme_info"` + inner + `},{"name":{"en":"Section","zh":"区块"},"settings":[]}]`
	if err := os.WriteFile(filepath.Join(dir, "config", "settings_schema.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

// TestReadThemeInfo_ArrayForm reproduces the real-theme bug: a genuine
// config/settings_schema.json is a JSON ARRAY whose theme_info block is the
// element with name=="theme_info". readThemeInfo must extract
// theme_name/theme_version from it (it previously errored "cannot unmarshal
// array").
func TestReadThemeInfo_ArrayForm(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	// Mirror real themes: some section "name" values are localized objects, not
	// strings. Parsing must tolerate mixed name types.
	content := `[
	  {"name":"theme_info","theme_name":"Nova","theme_version":"2.3.0","theme_author":"acme"},
	  {"name":{"en":"Colors","zh":"颜色"},"settings":[{"id":"bg","type":"color"}]}
	]`
	if err := os.WriteFile(filepath.Join(dir, "config", "settings_schema.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	name, version, err := readThemeInfo(dir)
	if err != nil {
		t.Fatalf("readThemeInfo on real array form returned error: %v", err)
	}
	if name != "Nova" || version != "2.3.0" {
		t.Fatalf("got (name=%q, version=%q), want (Nova, 2.3.0)", name, version)
	}
}

// TestReadThemeInfo_ArrayWithoutThemeInfo: a valid array lacking a theme_info
// element degrades to the fallbacks (basename / "unknown"), never an error.
func TestReadThemeInfo_ArrayWithoutThemeInfo(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "settings_schema.json"),
		[]byte(`[{"name":"Colors","settings":[]}]`), 0o644); err != nil {
		t.Fatal(err)
	}
	name, version, err := readThemeInfo(dir)
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if name != filepath.Base(dir) || version != "unknown" {
		t.Fatalf("got (%q,%q), want (%q, unknown)", name, version, filepath.Base(dir))
	}
}

// zipNames returns the forward-slash entry names of a zip archive, for
// substring / equality assertions.
func zipNames(t *testing.T, zipPath string) []string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("zip.OpenReader %s: %v", zipPath, err)
	}
	defer r.Close()
	out := make([]string, 0, len(r.File))
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	return out
}

// extractPackageEnvelope mirrors the helper in internal/theme/errors_test.go
// but lives here because that helper is in a different package.
func extractPackageEnvelope(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatal("err is nil")
	}
	type enveloper interface {
		Envelope() map[string]any
	}
	if e, ok := err.(enveloper); ok {
		return e.Envelope()
	}
	t.Fatalf("err does not expose Envelope(): %T", err)
	return nil
}

func TestPackage_FilenameFromThemeInfo(t *testing.T) {
	tmp := t.TempDir()
	makeThemeAt(t, tmp)
	writeSettings(t, tmp, "NoirChic", "1.4.2")
	t.Chdir(tmp)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(false),
		Tool:   "package",
		DryRun: false,
	}
	res, err := packageShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	wantZip := filepath.Join(tmp, "NoirChic-1.4.2.zip")
	if _, err := os.Stat(wantZip); err != nil {
		t.Fatalf("expected zip at %s: %v", wantZip, err)
	}
	if res.Body == nil {
		t.Fatal("Body should not be nil")
	}
	if res.Body["zip_path"] != wantZip {
		t.Errorf("Body.zip_path = %v, want %s", res.Body["zip_path"], wantZip)
	}
	if res.Body["name"] != "NoirChic" {
		t.Errorf("Body.name = %v, want NoirChic", res.Body["name"])
	}
	if res.Body["version"] != "1.4.2" {
		t.Errorf("Body.version = %v, want 1.4.2", res.Body["version"])
	}
}

func TestPackage_FallbackNameToCwd(t *testing.T) {
	parent := t.TempDir()
	dir := filepath.Join(parent, "my-shop")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	makeThemeAt(t, dir)
	writeSettings(t, dir, "", "2.0.0") // version only, no name
	t.Chdir(dir)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(false),
		Tool:   "package",
		DryRun: false,
	}
	res, err := packageShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	wantZip := filepath.Join(dir, "my-shop-2.0.0.zip")
	if _, err := os.Stat(wantZip); err != nil {
		t.Fatalf("expected zip at %s: %v", wantZip, err)
	}
	if res.Body["name"] != "my-shop" {
		t.Errorf("Body.name = %v, want my-shop (cwd basename)", res.Body["name"])
	}
}

func TestPackage_FallbackVersionToUnknown(t *testing.T) {
	tmp := t.TempDir()
	makeThemeAt(t, tmp)
	writeSettings(t, tmp, "BareName", "") // name only, no version
	t.Chdir(tmp)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(false),
		Tool:   "package",
		DryRun: false,
	}
	res, err := packageShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	wantZip := filepath.Join(tmp, "BareName-unknown.zip")
	if _, err := os.Stat(wantZip); err != nil {
		t.Fatalf("expected zip at %s: %v", wantZip, err)
	}
	if res.Body["version"] != "unknown" {
		t.Errorf("Body.version = %v, want unknown (fallback)", res.Body["version"])
	}
}

func TestPackage_SettingsMissingExitsValidation(t *testing.T) {
	tmp := t.TempDir()
	makeThemeAt(t, tmp) // theme dirs exist, but no config/settings_schema.json
	t.Chdir(tmp)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(false),
		Tool:   "package",
		DryRun: false,
	}
	_, err := packageShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("expected validation error when settings_schema.json missing")
	}
	env := extractPackageEnvelope(t, err)
	if env["type"] != "validation" {
		t.Errorf("envelope type = %v, want validation", env["type"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "does not look like a Shoplazza theme") {
		t.Errorf("envelope message missing expected hint; got %q", msg)
	}
}

func TestPackage_ThemeignoreAutoDetect(t *testing.T) {
	tmp := t.TempDir()
	makeThemeAt(t, tmp)
	writeSettings(t, tmp, "IgnoreTest", "1.0.0")
	// Exclude assets/main.css via .themeignore at root.
	if err := os.WriteFile(filepath.Join(tmp, ".themeignore"), []byte("assets/main.css\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(false),
		Tool:   "package",
		DryRun: false,
	}
	res, err := packageShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	zipPath, _ := res.Body["zip_path"].(string)
	if zipPath == "" {
		t.Fatal("Body.zip_path empty")
	}
	for _, name := range zipNames(t, zipPath) {
		if name == "assets/main.css" {
			t.Fatalf(".themeignore should have excluded assets/main.css; zip contains %v", zipNames(t, zipPath))
		}
	}
}

func TestPackage_NoIgnoreForcesV1Behavior(t *testing.T) {
	tmp := t.TempDir()
	makeThemeAt(t, tmp)
	writeSettings(t, tmp, "IgnoreTest", "1.0.0")
	// Same .themeignore as above — but with --no-ignore main.css must ship.
	if err := os.WriteFile(filepath.Join(tmp, ".themeignore"), []byte("assets/main.css\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(tmp)

	in := common.ExecInput{
		Flags:  flagsWithNoIgnore(true),
		Tool:   "package",
		DryRun: false,
	}
	res, err := packageShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	zipPath, _ := res.Body["zip_path"].(string)
	if zipPath == "" {
		t.Fatal("Body.zip_path empty")
	}
	found := false
	for _, name := range zipNames(t, zipPath) {
		if name == "assets/main.css" {
			found = true
			break
		}
	}
	if !found {
		t.Fatalf("with --no-ignore, assets/main.css should be in zip; have %v", zipNames(t, zipPath))
	}
}

// TestNoShortcutForPublishOrDelete is a guard ensuring +publish and +delete
// are NEVER implemented as workflow shortcuts (spec contract: Task C9).
func TestNoShortcutForPublishOrDelete(t *testing.T) {
	_, thisFile, _, _ := runtime.Caller(0)
	dir := filepath.Dir(thisFile)
	for _, banned := range []string{"publish.go", "delete.go"} {
		p := filepath.Join(dir, banned)
		if _, err := os.Stat(p); err == nil {
			t.Fatalf("%s exists; +publish/+delete must NOT be workflow shortcuts (spec contract)", p)
		}
	}
}

// ── themeZipName / sanitizeFileComponent ──────────────────────────────────────

// TestThemeZipName_SanitizesComponents: theme_name/theme_version come from a
// user-editable JSON file; separators and control chars must never reach the
// zip path (a name like "../../x" previously escaped the cwd).
func TestThemeZipName_SanitizesComponents(t *testing.T) {
	cases := []struct {
		name, version, want string
	}{
		{"Nova", "1.0.0", "Nova-1.0.0.zip"},
		{"../../evil", "1.0", ".._.._evil-1.0.zip"},
		{`a\b/c`, "2", "a_b_c-2.zip"},
		{"a:b", "v", "a_b-v.zip"},
		{"..", "..", "theme-theme.zip"},
		{"", "", "theme-theme.zip"},
		{"x\n", "1", "x_-1.zip"},
	}
	for _, c := range cases {
		if got := themeZipName(c.name, c.version); got != c.want {
			t.Errorf("themeZipName(%q,%q) = %q, want %q", c.name, c.version, got, c.want)
		}
	}
}

func TestSanitizeFileComponent_NoSeparatorsSurvive(t *testing.T) {
	for _, in := range []string{"../..", "a/b/c", `C:\x`, "a\x00b", "  "} {
		out := sanitizeFileComponent(in)
		if strings.ContainsAny(out, `/\`) {
			t.Errorf("sanitizeFileComponent(%q) = %q still has separators", in, out)
		}
		if out == "" || out == "." || out == ".." {
			t.Errorf("sanitizeFileComponent(%q) = %q is not a safe filename", in, out)
		}
	}
}
