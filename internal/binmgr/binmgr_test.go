package binmgr

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func TestEnsure_DownloadsThenCaches(t *testing.T) {
	cacheRoot := t.TempDir()
	downloads := 0
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		downloads++
		_, _ = w.Write([]byte("#!/bin/sh\necho fake-binary\n"))
	}))
	defer srv.Close()

	spec := Spec{
		Name:    "faketool",
		Version: "1.2.3",
		URL:     func(goos, goarch string) (string, error) { return srv.URL + "/faketool", nil },
	}

	p1, err := ensureIn(context.Background(), cacheRoot, spec)
	if err != nil {
		t.Fatalf("first ensure: %v", err)
	}
	if _, statErr := os.Stat(p1); statErr != nil {
		t.Fatalf("binary not present: %v", statErr)
	}
	// expected layout: <root>/faketool-1.2.3/faketool
	wantDir := filepath.Join(cacheRoot, "faketool-1.2.3")
	if filepath.Dir(p1) != wantDir {
		t.Fatalf("dir = %s, want %s", filepath.Dir(p1), wantDir)
	}
	// Second ensure must hit the cache, not re-download.
	if _, err := ensureIn(context.Background(), cacheRoot, spec); err != nil {
		t.Fatal(err)
	}
	if downloads != 1 {
		t.Fatalf("expected 1 download (cached on 2nd), got %d", downloads)
	}
}

// TestEnsure_ChecksumMatch_Succeeds verifies that a Spec whose SHA256 matches the
// served artifact downloads and caches normally.
func TestEnsure_ChecksumMatch_Succeeds(t *testing.T) {
	cacheRoot := t.TempDir()
	body := []byte("#!/bin/sh\necho verified\n")
	sum := sha256.Sum256(body)
	want := hex.EncodeToString(sum[:])
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(body)
	}))
	defer srv.Close()

	spec := Spec{
		Name:    "verifytool",
		Version: "1.0.0",
		URL:     func(goos, goarch string) (string, error) { return srv.URL + "/verifytool", nil },
		SHA256:  func(goos, goarch string) (string, error) { return want, nil },
	}
	p, err := ensureIn(context.Background(), cacheRoot, spec)
	if err != nil {
		t.Fatalf("ensure with matching checksum: %v", err)
	}
	if _, statErr := os.Stat(p); statErr != nil {
		t.Fatalf("binary not present after verified download: %v", statErr)
	}
}

// TestEnsure_ChecksumMismatch_Fails verifies that a tampered/served-vs-pinned
// mismatch is an internal-class (security) failure and no binary is cached.
func TestEnsure_ChecksumMismatch_Fails(t *testing.T) {
	cacheRoot := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("tampered payload"))
	}))
	defer srv.Close()

	spec := Spec{
		Name:    "verifytool",
		Version: "1.0.0",
		URL:     func(goos, goarch string) (string, error) { return srv.URL + "/verifytool", nil },
		SHA256: func(goos, goarch string) (string, error) {
			return "0000000000000000000000000000000000000000000000000000000000000000", nil
		},
	}
	p, err := ensureIn(context.Background(), cacheRoot, spec)
	if err == nil {
		t.Fatal("expected an integrity error on checksum mismatch, got nil")
	}
	var ce *classifiedError
	if errors.As(err, &ce) && ce.network {
		t.Errorf("checksum mismatch should be internal-class, got network: %v", err)
	}
	if p != "" {
		if _, statErr := os.Stat(p); statErr == nil {
			t.Errorf("binary must not be cached after a failed integrity check: %s", p)
		}
	}
}

// TestEnsure_EmptyChecksum_FailsClosed verifies that when a Spec provides a
// SHA256 func that yields an empty hash for the platform, Ensure refuses rather
// than silently exec'ing an unverified binary.
func TestEnsure_EmptyChecksum_FailsClosed(t *testing.T) {
	cacheRoot := t.TempDir()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte("whatever"))
	}))
	defer srv.Close()

	spec := Spec{
		Name:    "verifytool",
		Version: "1.0.0",
		URL:     func(goos, goarch string) (string, error) { return srv.URL + "/verifytool", nil },
		SHA256:  func(goos, goarch string) (string, error) { return "", nil }, // no pinned hash
	}
	if _, err := ensureIn(context.Background(), cacheRoot, spec); err == nil {
		t.Fatal("expected fail-closed error when SHA256 yields empty, got nil")
	}
}

func TestEnsure_404ReturnsNetworkError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
	}))
	defer srv.Close()

	spec := Spec{
		Name:    "missingtool",
		Version: "0.0.1",
		URL:     func(goos, goarch string) (string, error) { return srv.URL + "/notfound", nil },
	}

	_, xerr := Ensure(context.Background(), spec)
	if xerr == nil {
		t.Fatal("expected error for 404, got nil")
	}
	if xerr.Detail == nil || xerr.Detail.Type != "network" {
		t.Fatalf("expected network-type error, got: %+v", xerr)
	}
}

// TestEnsure_GzipDecompresses verifies that a Spec with Compression:"gzip"
// decompresses the downloaded bytes before writing the cached binary.
func TestEnsure_GzipDecompresses(t *testing.T) {
	const plainPayload = "#!/bin/sh\necho fake-gzip-binary\n"

	// Build a gzip-compressed payload.
	var gzBuf bytes.Buffer
	gw := gzip.NewWriter(&gzBuf)
	_, _ = gw.Write([]byte(plainPayload))
	_ = gw.Close()
	compressed := gzBuf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		// Serve raw gzip bytes WITHOUT Content-Encoding header so the HTTP
		// client does NOT transparently decompress them.
		_, _ = w.Write(compressed)
	}))
	defer srv.Close()

	spec := Spec{
		Name:        "gziptool",
		Version:     "9.9.9",
		Compression: "gzip",
		URL:         func(goos, goarch string) (string, error) { return srv.URL + "/gziptool.gz", nil },
	}

	cacheRoot := t.TempDir()
	p, err := ensureIn(context.Background(), cacheRoot, spec)
	if err != nil {
		t.Fatalf("ensureIn: %v", err)
	}
	got, readErr := os.ReadFile(p)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != plainPayload {
		t.Fatalf("got %q, want %q", string(got), plainPayload)
	}
}

// TestEnsure_URLResolutionError_IsInternal verifies a URL() resolution failure
// is classified as internal (exit 5), not network (exit 4).
func TestEnsure_URLResolutionError_IsInternal(t *testing.T) {
	urlErr := errors.New("unsupported platform: plan9/mips")
	spec := Spec{
		Name:    "badplatform",
		Version: "1.0.0",
		URL:     func(goos, goarch string) (string, error) { return "", urlErr },
	}

	_, xerr := Ensure(context.Background(), spec)
	if xerr == nil {
		t.Fatal("expected error for URL resolution failure, got nil")
	}
	if xerr.Code != output.ExitInternal {
		t.Fatalf("expected exit code %d (internal), got %d; detail: %+v",
			output.ExitInternal, xerr.Code, xerr.Detail)
	}
	if xerr.Detail == nil || xerr.Detail.Type != output.TypeInternal {
		t.Fatalf("expected internal-type error detail, got: %+v", xerr.Detail)
	}
}

// TestEnsure_TgzExtractsNamedBinary verifies that a Spec with Compression:"tgz"
// extracts the tar.gz and writes the entry whose base name matches Spec.Name
// (mirrors the darwin cloudflared .tgz which contains a file named "cloudflared").
func TestEnsure_TgzExtractsNamedBinary(t *testing.T) {
	const payload = "#!/bin/sh\necho fake-cloudflared\n"

	// Build a tar.gz containing several entries; only "cloudflared" should win.
	var buf bytes.Buffer
	gw := gzip.NewWriter(&buf)
	tw := tar.NewWriter(gw)
	entries := []struct {
		name string
		body string
	}{
		{"README.md", "ignore me\n"},
		{"cloudflared", payload},
		{"LICENSE", "ignore me too\n"},
	}
	for _, e := range entries {
		hdr := &tar.Header{Name: e.name, Mode: 0o755, Size: int64(len(e.body)), Typeflag: tar.TypeReg}
		if err := tw.WriteHeader(hdr); err != nil {
			t.Fatalf("tar WriteHeader: %v", err)
		}
		if _, err := tw.Write([]byte(e.body)); err != nil {
			t.Fatalf("tar Write: %v", err)
		}
	}
	if err := tw.Close(); err != nil {
		t.Fatalf("tar Close: %v", err)
	}
	if err := gw.Close(); err != nil {
		t.Fatalf("gzip Close: %v", err)
	}
	archive := buf.Bytes()

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write(archive)
	}))
	defer srv.Close()

	spec := Spec{
		Name:        "cloudflared",
		Version:     "2024.8.2",
		Compression: "tgz",
		URL:         func(goos, goarch string) (string, error) { return srv.URL + "/cloudflared-darwin-arm64.tgz", nil },
	}

	cacheRoot := t.TempDir()
	p, err := ensureIn(context.Background(), cacheRoot, spec)
	if err != nil {
		t.Fatalf("ensureIn: %v", err)
	}
	got, readErr := os.ReadFile(p)
	if readErr != nil {
		t.Fatalf("ReadFile: %v", readErr)
	}
	if string(got) != payload {
		t.Fatalf("got %q, want %q", string(got), payload)
	}
	// The extracted binary must be executable (POSIX file mode; Windows has no
	// executable bit).
	if runtime.GOOS != "windows" {
		fi, statErr := os.Stat(p)
		if statErr != nil {
			t.Fatalf("Stat: %v", statErr)
		}
		if fi.Mode().Perm()&0o100 == 0 {
			t.Fatalf("extracted binary not executable: mode %v", fi.Mode())
		}
	}
}
