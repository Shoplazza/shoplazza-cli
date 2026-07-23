package theme_extension

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/doc"
)

// recordDevDocMethods spins a server that records every method hitting the
// dev-doc endpoint and answers {}.
func recordDevDocMethods(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	methods := &[]string{}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		*methods = append(*methods, r.Method)
		_, _ = w.Write([]byte(`{}`))
	}))
	t.Cleanup(srv.Close)
	return srv, methods
}

// TestSyncDevDocFile_BinaryContentSkipped: invalid UTF-8 (binary assets) must
// NEVER ride the JSON dev-doc body — every non-UTF-8 byte would be mangled to
// U+FFFD on the wire (live-verified). The change is skipped, NO dev-doc call
// is made, and a one-line [skip] warning points at the bundle-upload path
// (mirrors themes serve's handleSync guard).
func TestSyncDevDocFile_BinaryContentSkipped(t *testing.T) {
	themeApp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(themeApp, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0xff, 0xfe, 0x00, 0x80} // PNG-ish, invalid UTF-8
	if err := os.WriteFile(filepath.Join(themeApp, "assets", "logo.png"), binary, 0o644); err != nil {
		t.Fatal(err)
	}

	srv, methods := recordDevDocMethods(t)
	var out, errLog strings.Builder
	for _, op := range []string{"create", "update"} {
		syncDevDocFile(context.Background(), client.New(srv.URL), "tex_1", themeApp,
			op, "assets/logo.png", doc.NewDeduper(), &out, &errLog)
	}

	if len(*methods) != 0 {
		t.Errorf("binary file must not hit the dev-doc API; methods=%v", *methods)
	}
	if !strings.Contains(errLog.String(), "[skip]") || !strings.Contains(errLog.String(), "binary") {
		t.Errorf("expected a [skip] ... binary warning, got: %q", errLog.String())
	}
	if !strings.Contains(errLog.String(), "te build") {
		t.Errorf("warning should point at the bundle-upload path ('te build'), got: %q", errLog.String())
	}
	if strings.Contains(out.String(), "synced") {
		t.Errorf("skipped file must not be reported as synced: %q", out.String())
	}
}

// TestSyncDevDocFile_TextStillSyncs: the guard must not block normal text files.
func TestSyncDevDocFile_TextStillSyncs(t *testing.T) {
	themeApp := t.TempDir()
	if err := os.MkdirAll(filepath.Join(themeApp, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(themeApp, "assets", "x.css"), []byte("/* ok */"), 0o644); err != nil {
		t.Fatal(err)
	}
	srv, methods := recordDevDocMethods(t)
	var out, errLog strings.Builder
	syncDevDocFile(context.Background(), client.New(srv.URL), "tex_1", themeApp,
		"update", "assets/x.css", doc.NewDeduper(), &out, &errLog)
	if len(*methods) != 1 || (*methods)[0] != http.MethodPatch {
		t.Fatalf("expected one PATCH for a text file, got %v (stderr: %q)", *methods, errLog.String())
	}
	if !strings.Contains(out.String(), "synced (update): assets/x.css") {
		t.Errorf("expected synced line, got %q", out.String())
	}
}

// TestNewServeFilter_NestedPathSkippedWithNote: paths deeper than <dir>/<file>
// pass ParseThemeFile but DevDocTarget (v1 parity, pinned) would derive an
// unknown type from the middle directory — the filter must reject them, with
// a single stderr note for the first occurrence.
func TestNewServeFilter_NestedPathSkippedWithNote(t *testing.T) {
	var note strings.Builder
	filter := newServeFilter(&note)

	if filter("assets/sub/img.png") {
		t.Fatal("nested path must be filtered out")
	}
	if !strings.Contains(note.String(), "assets/sub/img.png") || !strings.Contains(note.String(), "[skip]") {
		t.Fatalf("expected a one-line note naming the nested path, got %q", note.String())
	}
	before := note.Len()
	if filter("snippets/deep/x.liquid") {
		t.Fatal("nested path must be filtered out")
	}
	if note.Len() != before {
		t.Fatalf("note must be printed once, got more output: %q", note.String())
	}

	// flat paths keep flowing; build output and non-theme files stay filtered
	if !filter("assets/app.css") || !filter("blocks/foo.liquid") {
		t.Fatal("flat theme files must pass the filter")
	}
	if filter("assets/assets-manifest.json") {
		t.Fatal("assets-manifest.json is build output and must be filtered")
	}
	if filter("README.md") {
		t.Fatal("non-theme files must be filtered")
	}
}
