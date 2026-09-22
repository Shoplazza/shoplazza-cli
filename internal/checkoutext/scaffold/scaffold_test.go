package scaffold

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestProject_WritesTreeWithoutConfigJS(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "my-proj")
	if err := Project(dest, "my-proj", "my-ext"); err != nil {
		t.Fatalf("Project: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, ".gitignore")); err != nil {
		t.Errorf(".gitignore missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(dest, "_gitignore")); err == nil {
		t.Error("_gitignore must be renamed to .gitignore")
	}
	_ = filepath.Walk(dest, func(p string, info os.FileInfo, _ error) error {
		if info != nil && info.Name() == "extension.config.js" {
			t.Errorf("extension.config.js must NOT be generated: %s", p)
		}
		return nil
	})
	pkgRaw, _ := os.ReadFile(filepath.Join(dest, "package.json"))
	var pkg map[string]any
	_ = json.Unmarshal(pkgRaw, &pkg)
	if pkg["name"] != "my-proj" {
		t.Errorf("package.json name = %v, want my-proj", pkg["name"])
	}
	deps, _ := pkg["dependencies"].(map[string]any)
	if _, ok := deps["shoplazza-extension-ui"]; !ok {
		t.Error("dependencies must declare shoplazza-extension-ui (the checkout host SDK)")
	}
	if _, ok := deps["fs-extra"]; ok {
		t.Error("dependencies must not carry fs-extra: extensions run in the browser, it was dead weight inherited from v1")
	}
	extRaw, err := os.ReadFile(filepath.Join(dest, "extensions", "my-ext", "extension.json"))
	if err != nil {
		t.Fatalf("extension.json missing: %v", err)
	}
	var ext map[string]any
	_ = json.Unmarshal(extRaw, &ext)
	if ext["extensionName"] != "my-ext" {
		t.Errorf("extensionName = %v", ext["extensionName"])
	}
	if ext["extensionId"] != "" {
		t.Errorf("extensionId must stay empty until first push, got %v", ext["extensionId"])
	}
	if _, err := os.Stat(filepath.Join(dest, "extensions", "my-ext", "src", "index.js")); err != nil {
		t.Errorf("src/index.js missing: %v", err)
	}
}

// TestProject_FailureRemovesCreatedDir: a partial scaffold must not leave a
// half-written project behind when Project itself created the target dir.
func TestProject_FailureRemovesCreatedDir(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "half-proj")
	// A NUL byte in the extension name makes the extension scaffold fail
	// after the project dir and top-level files were already written.
	if err := Project(dest, "half-proj", "bad\x00ext"); err == nil {
		t.Fatal("expected Project to fail on an invalid extension name")
	}
	if _, err := os.Stat(dest); !os.IsNotExist(err) {
		t.Fatalf("failed Project must remove the dir it created, stat err = %v", err)
	}
}

// TestProject_FailureKeepsPreexistingDir: callers (checkout create) pre-check
// that the target does not exist; if it DID exist, a failed scaffold must not
// delete it.
func TestProject_FailureKeepsPreexistingDir(t *testing.T) {
	root := t.TempDir()
	dest := filepath.Join(root, "existing")
	if err := os.MkdirAll(dest, 0o755); err != nil {
		t.Fatal(err)
	}
	keep := filepath.Join(dest, "keep.txt")
	if err := os.WriteFile(keep, []byte("k"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := Project(dest, "existing", "bad\x00ext"); err == nil {
		t.Fatal("expected Project to fail")
	}
	if _, err := os.Stat(keep); err != nil {
		t.Fatalf("a pre-existing dir must survive a failed scaffold: %v", err)
	}
}

// skipIfDirWritable skips the test when a write into dir still succeeds despite a
// prior chmod 0o555 (running as root, or a filesystem that ignores directory
// permissions), since the write-failure path can't be induced in that case.
func skipIfDirWritable(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, ".write-probe")
	if f, err := os.Create(probe); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		t.Skipf("%s is writable despite chmod 0o555 (root or a permissive filesystem); cannot exercise the write-failure path", dir)
	}
}

// TestExtension_FailureKeepsPreexistingDir: same contract at the extension
// level — Extension only removes the target dir if it created it.
func TestExtension_FailureKeepsPreexistingDir(t *testing.T) {
	root := t.TempDir()
	dstExtDir := filepath.Join(root, "extensions", "second")
	if err := os.MkdirAll(dstExtDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(dstExtDir, 0o555); err != nil { // file writes inside will fail
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dstExtDir, 0o755) })
	skipIfDirWritable(t, dstExtDir)
	if err := Extension(root, "second"); err == nil {
		t.Fatal("expected Extension to fail in a read-only target dir")
	}
	if _, err := os.Stat(dstExtDir); err != nil {
		t.Fatalf("pre-existing extension dir must not be removed: %v", err)
	}
}

func TestExtension_AddsIntoExistingProject(t *testing.T) {
	root := t.TempDir()
	if err := os.MkdirAll(filepath.Join(root, "extensions"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := Extension(root, "second"); err != nil {
		t.Fatalf("Extension: %v", err)
	}
	if _, err := os.Stat(filepath.Join(root, "extensions", "second", "src", "index.js")); err != nil {
		t.Errorf("second extension not scaffolded: %v", err)
	}
}
