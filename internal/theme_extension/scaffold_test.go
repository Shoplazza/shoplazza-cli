package theme_extension

import (
	"os"
	"path/filepath"
	"testing"
)

func TestScaffoldBasicFromEmbedded(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "my-te")
	if err := Scaffold(dest, "my-te", "basic"); err != nil {
		t.Fatal(err)
	}
	app := filepath.Join(dest, "theme-app")
	if fi, err := os.Stat(app); err != nil || !fi.IsDir() {
		t.Fatalf("expected theme-app/ wrapper: %v", err)
	}
	for _, p := range []string{
		filepath.Join(app, "blocks", "my-te.liquid"),
		filepath.Join(app, "snippets", "my-te.liquid"),
		filepath.Join(app, "assets", "my-te.css"),
	} {
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected renamed file %s: %v", p, err)
		}
	}
	if _, err := os.Stat(filepath.Join(dest, "extension.config.json")); err == nil {
		t.Error("extension.config.json should not exist in v2 te projects")
	}
}

func TestScaffoldEmbedRenamesCSSInSnippets(t *testing.T) {
	dest := filepath.Join(t.TempDir(), "e")
	if err := Scaffold(dest, "e", "embed"); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(dest, "theme-app", "snippets", "e_css.liquid")); err != nil {
		t.Errorf("embed css rename: %v", err)
	}
}
