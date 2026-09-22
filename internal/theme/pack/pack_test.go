package pack

import (
	"archive/zip"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"testing"
)

func setupThemeDir(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	files := map[string]string{
		"assets/main.css":        "/* main */",
		"assets/sub/img.png":     "PNG-data",
		"layout/theme.liquid":    "<html>",
		"config/settings.json":   "{}",
		"sections/header.liquid": "{%}",
		"snippets/foo.liquid":    "snip",
		"templates/index.liquid": "tmpl",
		"locales/en.json":        "{}",
		"blocks/x.liquid":        "blk",
		"README.md":              "outside-theme",
		".DS_Store":              "noise",
	}
	for rel, content := range files {
		full := filepath.Join(root, rel)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		if err := os.WriteFile(full, []byte(content), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return root
}

func zipEntries(t *testing.T, zipPath string) []string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatal(err)
	}
	defer r.Close()
	out := make([]string, 0, len(r.File))
	for _, f := range r.File {
		out = append(out, f.Name)
	}
	sort.Strings(out)
	return out
}

func TestPack_EnumeratesOnlyThemeDirs(t *testing.T) {
	root := setupThemeDir(t)
	out := filepath.Join(root, "test.zip")
	if _, err := Pack(root, out, PackOptions{}); err != nil {
		t.Fatalf("Pack err: %v", err)
	}
	got := zipEntries(t, out)
	for _, name := range got {
		if strings.HasPrefix(name, "README") || strings.HasPrefix(name, ".DS_Store") {
			t.Errorf("non-theme file leaked into zip: %s", name)
		}
	}
	mustContain := []string{
		"assets/main.css",
		"assets/sub/img.png",
		"layout/theme.liquid",
	}
	for _, want := range mustContain {
		found := false
		for _, n := range got {
			if n == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("zip missing entry %q; have %v", want, got)
		}
	}
}

func TestPack_NestedSubdirectoriesPreserved(t *testing.T) {
	root := setupThemeDir(t)
	out := filepath.Join(root, "test.zip")
	if _, err := Pack(root, out, PackOptions{}); err != nil {
		t.Fatalf("Pack err: %v", err)
	}
	entries := zipEntries(t, out)
	for _, e := range entries {
		if e == "assets/sub/img.png" {
			return
		}
	}
	t.Fatalf("nested file assets/sub/img.png missing from zip: %v", entries)
}

func TestPack_ZipEntriesUseForwardSlashOnAllOS(t *testing.T) {
	root := setupThemeDir(t)
	out := filepath.Join(root, "test.zip")
	if _, err := Pack(root, out, PackOptions{}); err != nil {
		t.Fatalf("Pack err: %v", err)
	}
	for _, e := range zipEntries(t, out) {
		if strings.Contains(e, "\\") {
			t.Errorf("zip entry contains backslash: %q", e)
		}
	}
}

func TestEnumerateThemeFiles_ReturnsForwardSlashRelativePaths(t *testing.T) {
	root := setupThemeDir(t)
	files, err := EnumerateThemeFiles(root)
	if err != nil {
		t.Fatalf("EnumerateThemeFiles err: %v", err)
	}
	for _, f := range files {
		if strings.Contains(f, "\\") {
			t.Errorf("rel path contains backslash: %q", f)
		}
		if filepath.IsAbs(f) {
			t.Errorf("rel path should be relative: %q", f)
		}
	}
}

func TestThemeDirs_ContainsExactlyEight(t *testing.T) {
	want := map[string]bool{
		"assets": true, "blocks": true, "config": true, "layout": true,
		"locales": true, "sections": true, "snippets": true, "templates": true,
	}
	if len(ThemeDirs) != len(want) {
		t.Fatalf("ThemeDirs len = %d, want %d", len(ThemeDirs), len(want))
	}
	for _, d := range ThemeDirs {
		if !want[d] {
			t.Errorf("unexpected theme dir: %s", d)
		}
	}
}
