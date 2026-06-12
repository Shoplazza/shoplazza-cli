package pack

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func makeTarball(t *testing.T, entries map[string]string) []byte {
	t.Helper()
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	for name, content := range entries {
		hdr := &tar.Header{
			Name:     name,
			Size:     int64(len(content)),
			Typeflag: tar.TypeReg,
			Mode:     0o644,
		}
		if strings.HasSuffix(name, "/") {
			hdr.Typeflag = tar.TypeDir
			hdr.Size = 0
		}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatal(err)
		}
		if hdr.Typeflag == tar.TypeReg {
			_, _ = tw.Write([]byte(content))
		}
	}
	_ = tw.Close()
	_ = gz.Close()
	return buf.Bytes()
}

func TestCloneTemplate_StripsTopLevelDir(t *testing.T) {
	tarball := makeTarball(t, map[string]string{
		"Shoplazza-Nova-2023-abc1234/":                     "",
		"Shoplazza-Nova-2023-abc1234/config/":              "",
		"Shoplazza-Nova-2023-abc1234/config/settings.json": "{}",
		"Shoplazza-Nova-2023-abc1234/layout/theme.liquid":  "<html>",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	target := t.TempDir()
	if err := CloneTemplate(context.Background(), target); err != nil {
		t.Fatalf("CloneTemplate err: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config", "settings.json")); err != nil {
		t.Errorf("config/settings.json missing: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "Shoplazza-Nova-2023-abc1234")); err == nil {
		t.Errorf("top-level dir should have been stripped")
	}
}

func TestCloneTemplate_RejectsPathTraversal(t *testing.T) {
	tarball := makeTarball(t, map[string]string{
		"Shoplazza-Nova-2023-abc/../../etc/passwd": "evil",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	target := t.TempDir()
	err := CloneTemplate(context.Background(), target)
	if err == nil {
		t.Fatalf("expected path-traversal error")
	}
	if !strings.Contains(err.Error(), "unsafe") {
		t.Errorf("error should explain unsafe path: %v", err)
	}
	// Confirm no file was actually written outside the target dir.
	parent := filepath.Dir(target)
	leaked := filepath.Join(parent, "etc", "passwd")
	if _, err := os.Stat(leaked); err == nil {
		t.Fatalf("file leaked outside target: %s", leaked)
	}
}

func TestCloneTemplate_RejectsOver100MB(t *testing.T) {
	// Build a tarball that declares an entry > 100MB. We compress 100MB+1 of zeros (gzip-friendly).
	big := bytes.Repeat([]byte{0}, 101*1024*1024)
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: "Shoplazza-Nova-2023-abc/big.bin", Size: int64(len(big)), Typeflag: tar.TypeReg, Mode: 0o644}
	_ = tw.WriteHeader(hdr)
	_, _ = tw.Write(big)
	_ = tw.Close()
	_ = gz.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = io.Copy(w, bytes.NewReader(buf.Bytes()))
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	err := CloneTemplate(context.Background(), t.TempDir())
	if err == nil || !strings.Contains(err.Error(), "too large") && !strings.Contains(err.Error(), "100") {
		t.Fatalf("expected size-cap error, got: %v", err)
	}
}

func TestCloneTemplate_NetworkFailureWrapsError(t *testing.T) {
	old := templateTarballURL
	templateTarballURL = "http://127.0.0.1:1" // refused
	defer func() { templateTarballURL = old }()
	err := CloneTemplate(context.Background(), t.TempDir())
	if err == nil || errors.Is(err, io.EOF) {
		t.Fatalf("expected network error, got: %v", err)
	}
}

func TestCloneTemplate_IgnoresPaxGlobalHeaderForPrefixDetection(t *testing.T) {
	// Synthetic tarball: first entry is pax_global_header (extended), then the real top-dir entries.
	buf := bytes.NewBuffer(nil)
	gz := gzip.NewWriter(buf)
	tw := tar.NewWriter(gz)
	// pax_global_header: zero-size, TypeXGlobalHeader, name conventionally "pax_global_header"
	_ = tw.WriteHeader(&tar.Header{
		Name:     "pax_global_header",
		Size:     0,
		Typeflag: tar.TypeXGlobalHeader,
		Mode:     0o644,
	})
	// real top dir + entry
	_ = tw.WriteHeader(&tar.Header{Name: "Shoplazza-Nova-2023-abc/", Typeflag: tar.TypeDir, Mode: 0o755})
	_ = tw.WriteHeader(&tar.Header{Name: "Shoplazza-Nova-2023-abc/config/settings.json", Size: 2, Typeflag: tar.TypeReg, Mode: 0o644})
	_, _ = tw.Write([]byte("{}"))
	_ = tw.Close()
	_ = gz.Close()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(buf.Bytes())
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	target := t.TempDir()
	if err := CloneTemplate(context.Background(), target); err != nil {
		t.Fatalf("CloneTemplate err: %v", err)
	}
	// settings.json must land at target/config/settings.json (top dir stripped)
	if _, err := os.Stat(filepath.Join(target, "config", "settings.json")); err != nil {
		t.Errorf("settings.json missing — pax_global_header poisoned top-prefix detection: %v", err)
	}
	// And NOT under target/Shoplazza-Nova-2023-abc/ nor target/pax_global_header/
	if _, err := os.Stat(filepath.Join(target, "Shoplazza-Nova-2023-abc")); err == nil {
		t.Errorf("real top dir was not stripped")
	}
	if _, err := os.Stat(filepath.Join(target, "pax_global_header")); err == nil {
		t.Errorf("pax_global_header should never become an output directory")
	}
}

// TestCloneTemplate_RefusesNonEmptyTargetDir verifies the guard fires before
// any network or extraction work and leaves the existing directory untouched.
func TestCloneTemplate_RefusesNonEmptyTargetDir(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(_ http.ResponseWriter, _ *http.Request) {
		t.Error("non-empty target must be rejected before any download")
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	target := t.TempDir()
	existing := filepath.Join(target, "layout")
	if err := os.MkdirAll(existing, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(existing, "theme.liquid"), []byte("precious"), 0o644); err != nil {
		t.Fatal(err)
	}

	err := CloneTemplate(context.Background(), target)
	if !errors.Is(err, ErrTargetDirNotEmpty) {
		t.Fatalf("expected ErrTargetDirNotEmpty, got: %v", err)
	}
	got, rerr := os.ReadFile(filepath.Join(existing, "theme.liquid"))
	if rerr != nil || string(got) != "precious" {
		t.Errorf("existing file must be untouched; content=%q err=%v", got, rerr)
	}
}

// TestCloneTemplate_EmptyExistingTargetDirProceeds verifies an existing but
// empty directory is accepted.
func TestCloneTemplate_EmptyExistingTargetDirProceeds(t *testing.T) {
	tarball := makeTarball(t, map[string]string{
		"Shoplazza-Nova-2023-abc/config/settings.json": "{}",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(tarball)
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	target := t.TempDir() // exists and is empty
	if err := CloneTemplate(context.Background(), target); err != nil {
		t.Fatalf("empty existing dir should proceed: %v", err)
	}
	if _, err := os.Stat(filepath.Join(target, "config", "settings.json")); err != nil {
		t.Errorf("expected extraction into the empty dir: %v", err)
	}
}

// TestCloneTemplate_Non200ReturnsTemplateDownloadSentinel verifies a non-200
// response surfaces as the typed ErrTemplateDownload sentinel.
func TestCloneTemplate_Non200ReturnsTemplateDownloadSentinel(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "rate limited", http.StatusForbidden)
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	err := CloneTemplate(context.Background(), t.TempDir())
	if !errors.Is(err, ErrTemplateDownload) {
		t.Fatalf("expected ErrTemplateDownload sentinel, got: %v", err)
	}
	if !strings.Contains(err.Error(), "403") {
		t.Errorf("error should carry the HTTP status: %v", err)
	}
}

// TestCloneTemplate_ContextCancelAborts verifies the download honors the caller's ctx.
func TestCloneTemplate_ContextCancelAborts(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel() // already canceled

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(makeTarball(t, map[string]string{"a/x": "y"}))
	}))
	defer srv.Close()
	old := templateTarballURL
	templateTarballURL = srv.URL + "/tarball"
	defer func() { templateTarballURL = old }()

	err := CloneTemplate(ctx, t.TempDir())
	if err == nil || !errors.Is(err, context.Canceled) {
		t.Fatalf("expected context.Canceled, got: %v", err)
	}
}

func TestTemplateTarballURL_DefaultMatchesNova2023Main(t *testing.T) {
	if TemplateTarballURL != "https://codeload.github.com/Shoplazza/Nova-2023/tar.gz/refs/heads/main" {
		t.Fatalf("TemplateTarballURL changed: %s", TemplateTarballURL)
	}
}
