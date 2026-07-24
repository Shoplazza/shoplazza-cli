package app

import (
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// TestScanLocalExtensions_PopulatesExtensionID proves the scanner reads the
// toml `id` into LocalExt.ExtensionID so Diff's Pass-1 id-match can fire.
func TestScanLocalExtensions_PopulatesExtensionID(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "co")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	toml := "id = \"ext1\"\nname = \"co\"\ntype = \"checkout\"\nversion = \"3.2.1\"\n"
	if err := os.WriteFile(filepath.Join(extDir, "shoplazza.extension.toml"), []byte(toml), 0o644); err != nil {
		t.Fatal(err)
	}

	locals, err := ScanLocalExtensions(root)
	if err != nil {
		t.Fatalf("ScanLocalExtensions: %v", err)
	}
	if len(locals) != 1 {
		t.Fatalf("locals = %+v, want 1", locals)
	}
	got := locals[0]
	if got.ExtensionID != "ext1" {
		t.Fatalf("ExtensionID = %q, want ext1", got.ExtensionID)
	}
	if got.Name != "co" || got.Type != "checkout" || got.Version != "3.2.1" || got.Dir != "co" {
		t.Fatalf("LocalExt = %+v", got)
	}
}

// TestScanLocalExtensions_MissingDir tolerates an absent extensions/ dir.
func TestScanLocalExtensions_MissingDir(t *testing.T) {
	locals, err := ScanLocalExtensions(t.TempDir())
	if err != nil {
		t.Fatalf("ScanLocalExtensions: %v", err)
	}
	if locals != nil {
		t.Fatalf("locals = %+v, want nil", locals)
	}
}

// TestScanLocalExtensions_DirWithoutToml_Skipped pins the missing-file branch:
// toml.DecodeFile on an absent file yields a *fs.PathError satisfying
// os.ErrNotExist, and such dirs are skipped (not extension dirs), not errors.
func TestScanLocalExtensions_DirWithoutToml_Skipped(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "extensions", "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	locals, err := ScanLocalExtensions(root)
	if err != nil {
		t.Fatalf("ScanLocalExtensions: %v", err)
	}
	if len(locals) != 0 {
		t.Fatalf("locals = %+v, want none", locals)
	}
}

// TestScanLocalExtensions_V1Fallback: a dir with only the legacy v1
// extension.config.json (no v2 toml) is read and mapped to LocalExt, so v1
// projects deploy without a manual migration.
func TestScanLocalExtensions_V1Fallback(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "preorder")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	v1 := `{"extensionId":"657218221862028613","appId":"app_x","partnerId":"3665","extensionName":"preorder","version":"1.0.0","type":"theme","subtype":"basic"}`
	if err := os.WriteFile(filepath.Join(extDir, "extension.config.json"), []byte(v1), 0o644); err != nil {
		t.Fatal(err)
	}
	locals, err := ScanLocalExtensions(root)
	if err != nil {
		t.Fatalf("ScanLocalExtensions: %v", err)
	}
	if len(locals) != 1 {
		t.Fatalf("locals = %+v, want 1", locals)
	}
	got := locals[0]
	if got.ExtensionID != "657218221862028613" || got.Name != "preorder" || got.Type != "theme" || got.Version != "1.0.0" || got.AppID != "app_x" {
		t.Fatalf("v1 mapping wrong: %+v", got)
	}
}

// TestScanLocalExtensions_V2TomlWinsOverV1: when both formats are present, the
// v2 toml is authoritative and the v1 json is ignored.
func TestScanLocalExtensions_V2TomlWinsOverV1(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "co")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "shoplazza.extension.toml"),
		[]byte("id = \"v2id\"\nname = \"co\"\ntype = \"checkout\"\nversion = \"2.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "extension.config.json"),
		[]byte(`{"extensionId":"v1id","extensionName":"co","type":"checkout","version":"1.0.0"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	locals, err := ScanLocalExtensions(root)
	if err != nil {
		t.Fatalf("ScanLocalExtensions: %v", err)
	}
	if len(locals) != 1 || locals[0].ExtensionID != "v2id" || locals[0].AppID != "" {
		t.Fatalf("toml should win: %+v", locals)
	}
}

// TestScanLocalExtensions_V1Malformed_Validation: a present but unparseable v1
// json surfaces as a validation error naming the file, like the toml path.
func TestScanLocalExtensions_V1Malformed_Validation(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "bad")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "extension.config.json"), []byte("{not json"), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := ScanLocalExtensions(root)
	if err == nil {
		t.Fatal("expected validation error for malformed v1 config")
	}
	if err.Code != output.ExitValidation || !strings.Contains(err.Error(), "extensions/bad/extension.config.json") {
		t.Fatalf("error should name the v1 file as validation, got %q (code %d)", err.Error(), err.Code)
	}
}

// TestScanLocalExtensions_MalformedToml_Validation: a present but
// unparseable extension toml must surface as a validation error naming the
// file — silently skipping it made deploy/dev quietly ignore the extension.
func TestScanLocalExtensions_MalformedToml_Validation(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "broken")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "shoplazza.extension.toml"),
		[]byte("name = \"unterminated\ntype =\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	_, err := ScanLocalExtensions(root)
	if err == nil {
		t.Fatal("expected a validation error for a malformed extension toml")
	}
	if err.Code != output.ExitValidation {
		t.Fatalf("exit code = %d, want ExitValidation (%d)", err.Code, output.ExitValidation)
	}
	if !strings.Contains(err.Error(), "extensions/broken/shoplazza.extension.toml") {
		t.Fatalf("error should name the file, got %q", err.Error())
	}
}
