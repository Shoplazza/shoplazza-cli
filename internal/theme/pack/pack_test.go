package pack

import (
	"archive/zip"
	"bytes"
	"errors"
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

func makeTestZip(t *testing.T, entries map[string]string) string {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	zw := zip.NewWriter(buf)
	for name, content := range entries {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatal(err)
		}
		_, _ = w.Write([]byte(content))
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	out := filepath.Join(t.TempDir(), "test.zip")
	if err := os.WriteFile(out, buf.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return out
}

func TestUnpack_StripsTopLevelDir(t *testing.T) {
	zipPath := makeTestZip(t, map[string]string{
		"NoirChic-1.0/assets/main.css":     "css",
		"NoirChic-1.0/layout/theme.liquid": "liquid",
		"NoirChic-1.0/assets/sub/img.png":  "png",
	})
	target := t.TempDir()
	if err := Unpack(zipPath, target, UnpackOptions{StripTopDir: true}); err != nil {
		t.Fatalf("Unpack err: %v", err)
	}
	for _, rel := range []string{"assets/main.css", "layout/theme.liquid", "assets/sub/img.png"} {
		p := filepath.Join(target, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected file %s to exist after StripTopDir: %v", rel, err)
		}
	}
}

func TestUnpack_RejectsPathTraversal(t *testing.T) {
	zipPath := makeTestZip(t, map[string]string{
		"NoirChic/../../etc/passwd": "evil",
		"NoirChic/assets/main.css":  "css",
	})
	target := t.TempDir()
	err := Unpack(zipPath, target, UnpackOptions{StripTopDir: true, PathTraversalCheck: true})
	if err == nil {
		t.Fatalf("expected error for path traversal")
	}
	if !errors.Is(err, ErrUnsafeArchivePath) {
		t.Errorf("error must wrap ErrUnsafeArchivePath sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("error should mention 'unsafe': %v", err)
	}
	// The message must name the OFFENDING ENTRY so callers can surface it.
	if !strings.Contains(err.Error(), "etc/passwd") {
		t.Errorf("error should name the offending entry: %v", err)
	}
	// Confirm no file was actually written outside the target dir.
	parent := filepath.Dir(target)
	leaked := filepath.Join(parent, "etc", "passwd")
	if _, err := os.Stat(leaked); err == nil {
		t.Fatalf("file leaked outside target: %s", leaked)
	}
}

func TestUnpack_RejectsArchivesOverConfiguredLimit(t *testing.T) {
	// Construct an entry with declared uncompressed size >limit without actually writing 200MB.
	// We use a real but small zip; the limit check should be on cumulative copied bytes.
	zipPath := makeTestZip(t, map[string]string{
		"big.bin": strings.Repeat("x", 1024),
	})
	target := t.TempDir()
	// Set artificially small MaxTotalSize to trigger.
	err := Unpack(zipPath, target, UnpackOptions{MaxTotalSize: 100, PathTraversalCheck: true})
	if err == nil {
		t.Fatalf("expected size-limit error")
	}
	if !errors.Is(err, ErrSizeLimit) {
		t.Errorf("error must wrap ErrSizeLimit sentinel: %v", err)
	}
	if !strings.Contains(err.Error(), "exceeds") && !strings.Contains(err.Error(), "limit") {
		t.Errorf("error should describe size limit: %v", err)
	}
}

func TestUnpack_DefaultLimitIs200MB(t *testing.T) {
	// A small archive extracts under the zero-value default, exercising the
	// "MaxTotalSize <= 0 → defaultMaxUnpackSize" branch and pinning the constant.
	zipPath := makeTestZip(t, map[string]string{"x": strings.Repeat("a", 1024)})
	target := t.TempDir()
	if err := Unpack(zipPath, target, UnpackOptions{}); err != nil {
		t.Fatalf("1KB extract under default 200MB limit should succeed: %v", err)
	}
	if defaultMaxUnpackSize != 200*1024*1024 {
		t.Fatalf("defaultMaxUnpackSize changed: got %d, want %d", defaultMaxUnpackSize, 200*1024*1024)
	}
}

func TestUnpack_OverwritesExistingFiles(t *testing.T) {
	zipPath := makeTestZip(t, map[string]string{
		"assets/main.css": "new-content",
	})
	target := t.TempDir()
	old := filepath.Join(target, "assets", "main.css")
	_ = os.MkdirAll(filepath.Dir(old), 0o755)
	_ = os.WriteFile(old, []byte("old-content"), 0o644)
	if err := Unpack(zipPath, target, UnpackOptions{}); err != nil {
		t.Fatalf("Unpack err: %v", err)
	}
	got, _ := os.ReadFile(old)
	if string(got) != "new-content" {
		t.Errorf("file was not overwritten: %q", got)
	}
}

func TestUnpack_CorruptZipReturnsError(t *testing.T) {
	bad := filepath.Join(t.TempDir(), "corrupt.zip")
	_ = os.WriteFile(bad, []byte("not a zip"), 0o644)
	err := Unpack(bad, t.TempDir(), UnpackOptions{})
	if err == nil || !errors.Is(err, zip.ErrFormat) && !strings.Contains(err.Error(), "zip") {
		t.Fatalf("expected zip format error, got: %v", err)
	}
}
