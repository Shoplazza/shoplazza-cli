package pack

import (
	"archive/zip"
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

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
