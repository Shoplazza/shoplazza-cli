package app

import (
	"archive/zip"
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestZipExtension_PacksTree(t *testing.T) {
	src := t.TempDir()
	os.MkdirAll(filepath.Join(src, "blocks"), 0o755)
	os.WriteFile(filepath.Join(src, "shoplazza.extension.toml"), []byte("name=\"x\"\n"), 0o644)
	os.WriteFile(filepath.Join(src, "blocks", "a.liquid"), []byte("hello"), 0o644)
	// a .git dir must be excluded
	os.MkdirAll(filepath.Join(src, ".git"), 0o755)
	os.WriteFile(filepath.Join(src, ".git", "HEAD"), []byte("ref"), 0o644)

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
	found := map[string]bool{}
	for _, f := range zr.File {
		found[f.Name] = true
	}
	if !found["theme-app/shoplazza.extension.toml"] || !found["theme-app/blocks/a.liquid"] {
		t.Fatalf("zip missing expected theme-app/ prefixed entries: %v", found)
	}
	if found[".git/HEAD"] || found["theme-app/.git/HEAD"] {
		t.Fatalf(".git must be excluded from the zip")
	}
}

func TestThemeZipName_Format(t *testing.T) {
	src := t.TempDir()
	os.WriteFile(filepath.Join(src, "index.css"), []byte(".x{}"), 0o644)
	os.WriteFile(filepath.Join(src, "main.js"), []byte("export default {}"), 0o644)

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
	os.WriteFile(filepath.Join(src, "a.js"), []byte("content"), 0o644)

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
	os.WriteFile(filepath.Join(extDir, "main.js"), []byte("export default {}"), 0o644)

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
