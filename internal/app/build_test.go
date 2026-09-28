package app

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func writeTree(t *testing.T, root string, files ...string) {
	t.Helper()
	for _, f := range files {
		p := filepath.Join(root, filepath.FromSlash(f))
		if err := os.MkdirAll(filepath.Dir(p), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(p, []byte(f), 0o644); err != nil {
			t.Fatal(err)
		}
	}
}

func TestZipExtension_PacksTree(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src,
		"assets-manifest.json", "blocks/a.liquid", "locales/en-US.json",
		// skipped: root files, dot entries at any depth, node_modules
		"shoplazza.extension.toml", "package.json", "package-lock.json", "README.md",
		".gitignore", ".env", ".DS_Store", "blocks/.DS_Store", ".git/HEAD",
		"node_modules/x/index.js",
	)

	out := filepath.Join(t.TempDir(), "app-deploy", "x.zip")
	// theme leg passes "theme-app" so every entry is under that top dir (v1 parity).
	got, err := zipExtension(src, out, "theme-app")
	if err != nil {
		t.Fatalf("zipExtension: %v", err)
	}
	if got != out {
		t.Fatalf("returned path = %q, want %q", got, out)
	}

	zr, err := zip.OpenReader(out)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	var names []string
	for _, f := range zr.File {
		names = append(names, f.Name)
	}
	want := "theme-app/assets-manifest.json theme-app/blocks/a.liquid theme-app/locales/en-US.json"
	if got := strings.Join(names, " "); got != want {
		t.Fatalf("zip entries = %q, want %q", got, want)
	}
}

func TestThemeZipName_IgnoresSkippedFiles(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, "blocks/a.liquid")
	n1, err := themeZipName(src, "ext")
	if err != nil {
		t.Fatal(err)
	}
	writeTree(t, src, "README.md", ".DS_Store", "node_modules/x.js")
	n2, err := themeZipName(src, "ext")
	if err != nil {
		t.Fatal(err)
	}
	if n1[:12] != n2[:12] {
		t.Errorf("skipped files changed the hash: %q vs %q", n1, n2)
	}
	writeTree(t, src, "blocks/b.liquid")
	n3, err := themeZipName(src, "ext")
	if err != nil {
		t.Fatal(err)
	}
	if n1[:12] == n3[:12] {
		t.Errorf("bundled file did not change the hash: %q", n3)
	}
}

func TestThemeZipName_Format(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, "assets/index.css", "blocks/main.liquid")

	name, err := themeZipName(src, "mytheme")
	if err != nil {
		t.Fatalf("themeZipName: %v", err)
	}
	// Format: "<name>-<hash8><ts8>.zip"
	if !strings.HasPrefix(name, "mytheme-") {
		t.Errorf("expected name prefix 'mytheme-', got %q", name)
	}
	if !strings.HasSuffix(name, ".zip") {
		t.Errorf("expected .zip suffix, got %q", name)
	}
	// hash8 + ts8 = 16 hex chars between name- and .zip
	inner := strings.TrimPrefix(strings.TrimSuffix(name, ".zip"), "mytheme-")
	if len(inner) != 16 {
		t.Errorf("expected 16-char hash+ts, got %d chars in %q", len(inner), inner)
	}
}

func TestThemeZipName_Deterministic_SameContent(t *testing.T) {
	src := t.TempDir()
	writeTree(t, src, "blocks/a.liquid")

	n1, err := themeZipName(src, "ext")
	if err != nil {
		t.Fatalf("first call: %v", err)
	}
	n2, err := themeZipName(src, "ext")
	if err != nil {
		t.Fatalf("second call: %v", err)
	}
	// The hash portion (first 8 chars after "ext-") must be the same.
	h1 := strings.TrimPrefix(n1, "ext-")[:8]
	h2 := strings.TrimPrefix(n2, "ext-")[:8]
	if h1 != h2 {
		t.Errorf("hash unstable: %q vs %q", h1, h2)
	}
}

// TestBuildArtifactFor_Theme_V1NestedLayout: a v1 extension nests the theme
// content under theme-app/ (alongside extension.config.json). The zip must root
// the CONTENT at theme-app/ (so the backend finds theme-app/assets-manifest.json),
// not double-nest it (theme-app/theme-app/...) or include the v1 metadata.
func TestBuildArtifactFor_Theme_V1NestedLayout(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "preorder")
	themeApp := filepath.Join(extDir, "theme-app")
	os.MkdirAll(filepath.Join(themeApp, "blocks"), 0o755)
	os.WriteFile(filepath.Join(themeApp, "assets-manifest.json"), []byte("{}"), 0o644)
	os.WriteFile(filepath.Join(themeApp, "blocks", "a.liquid"), []byte("x"), 0o644)
	// v1 metadata at the extension root — must NOT end up in the theme bundle.
	os.WriteFile(filepath.Join(extDir, "extension.config.json"), []byte("{}"), 0o644)

	got, exitErr := BuildArtifactFor(context.Background(), root, LocalExt{
		Dir: "preorder", Name: "preorder", Type: "theme",
	}, false)
	if exitErr != nil {
		t.Fatalf("BuildArtifactFor: %v", exitErr)
	}
	zr, err := zip.OpenReader(got)
	if err != nil {
		t.Fatalf("open zip: %v", err)
	}
	defer zr.Close()
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found["theme-app/assets-manifest.json"] || !found["theme-app/blocks/a.liquid"] {
		t.Fatalf("content must be rooted at theme-app/: %v", found)
	}
	if found["theme-app/theme-app/assets-manifest.json"] {
		t.Fatalf("must not double-nest theme-app/: %v", found)
	}
	if found["theme-app/extension.config.json"] {
		t.Fatalf("v1 metadata must not be bundled: %v", found)
	}
}

func TestBuildArtifactFor_UnknownType_Errors(t *testing.T) {
	_, exitErr := BuildArtifactFor(context.Background(), t.TempDir(), LocalExt{
		Dir: "myext", Name: "myext", Type: "unknown-type",
	}, false)
	if exitErr == nil {
		t.Fatal("expected error for unknown extension type")
	}
	if !strings.Contains(exitErr.Error(), "unknown extension type") {
		t.Errorf("unexpected error message: %v", exitErr)
	}
}

func TestBuildArtifactFor_Theme_ProducesZip(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "mytheme")
	os.MkdirAll(extDir, 0o755)
	os.WriteFile(filepath.Join(extDir, "shoplazza.extension.toml"), []byte("name=\"mytheme\"\n"), 0o644)
	writeTree(t, extDir, "blocks/main.liquid")

	got, exitErr := BuildArtifactFor(context.Background(), root, LocalExt{
		Dir: "mytheme", Name: "mytheme", Type: "theme",
	}, false)
	if exitErr != nil {
		t.Fatalf("BuildArtifactFor theme: %v", exitErr)
	}
	if !strings.HasSuffix(got, ".zip") {
		t.Errorf("expected .zip artifact, got %q", got)
	}
	if _, err := os.Stat(got); err != nil {
		t.Errorf("zip file must exist at %q: %v", got, err)
	}
}
