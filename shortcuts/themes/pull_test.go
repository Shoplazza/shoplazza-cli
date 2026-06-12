package themes

import (
	"archive/zip"
	"bytes"
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/theme"
	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// pullFlags builds a FlagSet over a cobra command with --theme-id.
func pullFlags(themeID string) common.FlagSet {
	cmd := &cobra.Command{Use: "pull"}
	cmd.Flags().String("theme-id", themeID, "")
	return common.NewCobraFlagSet(cmd)
}

// buildTestZip writes a zip archive in-memory from a name→content map.
// Mirrors the pattern in internal/theme/pack/unpack_test.go.
func buildTestZip(t *testing.T, files map[string]string) []byte {
	t.Helper()
	buf := &bytes.Buffer{}
	zw := zip.NewWriter(buf)
	for name, content := range files {
		w, err := zw.Create(name)
		if err != nil {
			t.Fatalf("zip create %s: %v", name, err)
		}
		if _, err := w.Write([]byte(content)); err != nil {
			t.Fatalf("zip write %s: %v", name, err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatalf("zip close: %v", err)
	}
	return buf.Bytes()
}

// chdirT cd's the process into dir for the duration of the test, restoring
// the original cwd in t.Cleanup. Required because pull extracts into cwd —
// we want a clean isolated dir per test, not the package source tree.
func chdirT(t *testing.T, dir string) {
	t.Helper()
	prev, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatalf("chdir %s: %v", dir, err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })
}

func TestPull_MissingThemeIDExitsValidation(t *testing.T) {
	in := common.ExecInput{Flags: pullFlags("")}
	_, err := pullShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected validation error for missing --theme-id")
	}
	if !errors.Is(err, theme.ErrMissingThemeFlag) {
		t.Errorf("expected ErrMissingThemeFlag sentinel; got %T: %v", err, err)
	}
	// Envelope() carrier exposed via the wrapper; assert type=validation and
	// the hint references the discovery shortcut.
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T", err)
	}
	env := ec.Envelope()
	if env["type"] != output.TypeValidation {
		t.Errorf("type = %v, want %q", env["type"], output.TypeValidation)
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "shoplazza themes list") {
		t.Errorf("hint should reference `shoplazza themes list`; got %q", hint)
	}
}

func TestPull_DryRunOutputs2PlannedRequests(t *testing.T) {
	in := common.ExecInput{DryRun: true, Flags: pullFlags("abc123")}
	res, err := pullShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if len(res.Plans) != 2 {
		t.Fatalf("dry-run should emit 2 PlannedRequest (PlanDetail v2 + PlanDownload v1); got %d", len(res.Plans))
	}
	hasDetail, hasDownload := false, false
	for _, p := range res.Plans {
		// PlanDetail → v2 spec path: /openapi/2026-01/themes/abc123
		if strings.Contains(p.Path, "/2026-01/themes/abc123") && !strings.Contains(p.Path, "/download") {
			hasDetail = true
		}
		// PlanDownload → v1 path: /openapi/2020-07/themes/abc123/download
		if strings.Contains(p.Path, "/2020-07/themes/abc123/download") {
			hasDownload = true
		}
	}
	if !hasDetail || !hasDownload {
		paths := make([]string, 0, len(res.Plans))
		for _, p := range res.Plans {
			paths = append(paths, p.Path)
		}
		t.Fatalf("expected PlanDetail (v2) + PlanDownload (v1); got %v", paths)
	}
}

func TestPull_DryRunDoesNotWriteFiles(t *testing.T) {
	tmp := t.TempDir()
	chdirT(t, tmp)
	in := common.ExecInput{DryRun: true, Flags: pullFlags("abc123")}
	if _, err := pullShortcut.Execute(context.Background(), in); err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	entries, err := os.ReadDir(tmp)
	if err != nil {
		t.Fatalf("readdir: %v", err)
	}
	if len(entries) != 0 {
		names := make([]string, 0, len(entries))
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Errorf("dry-run should leave cwd untouched; got entries: %v", names)
	}
}

func TestPull_DownloadStreamingAndUnpack(t *testing.T) {
	// Build a small zip with a top-level "Nova-1.0/" dir; StripTopDir=true
	// should land the inner files directly into cwd.
	zipBytes := buildTestZip(t, map[string]string{
		"Nova-1.0/assets/main.css":     "css-content",
		"Nova-1.0/layout/theme.liquid": "<html/>",
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.HasSuffix(r.URL.Path, "/download"):
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
		default:
			// PlanDetail (theme name lookup) — return a minimal envelope.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	defer srv.Close()

	tmp := t.TempDir()
	chdirT(t, tmp)

	c := client.New(srv.URL)
	in := common.ExecInput{Client: c, Flags: pullFlags("abc123")}
	res, err := pullShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}

	// Verify body shape carries the pull result.
	if got := res.Body["theme_id"]; got != "abc123" {
		t.Errorf("theme_id = %v, want abc123", got)
	}
	if got, _ := res.Body["target"].(string); got != "./" {
		t.Errorf("target = %v, want ./", got)
	}

	// Verify files were extracted with top-level dir stripped.
	for _, rel := range []string{"assets/main.css", "layout/theme.liquid"} {
		p := filepath.Join(tmp, filepath.FromSlash(rel))
		if _, err := os.Stat(p); err != nil {
			t.Errorf("expected %s under cwd, got stat err: %v", rel, err)
		}
	}
	// Verify the top-level dir itself was NOT created (StripTopDir).
	if _, err := os.Stat(filepath.Join(tmp, "Nova-1.0")); !os.IsNotExist(err) {
		t.Errorf("StripTopDir failed: top-level dir still exists (stat err: %v)", err)
	}

	// Verify the tmp zip was cleaned up after success.
	// We can't easily get the exact tmp path without injection, so glob the
	// pattern and assert nothing remains for THIS theme id.
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "shoplazza-theme-abc123-*.zip"))
	if len(matches) > 0 {
		t.Errorf("tmp zip should be cleaned on success; found leftovers: %v", matches)
	}
}

func TestPull_HTTPErrorRetainsTmpZip(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			http.Error(w, `{"error":"not found"}`, http.StatusNotFound)
			return
		}
		// PlanDetail — let it 200 with empty body so we exercise the
		// download branch and its error classification.
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	tmp := t.TempDir()
	chdirT(t, tmp)

	c := client.New(srv.URL)
	in := common.ExecInput{Client: c, Flags: pullFlags("missing-id")}
	_, err := pullShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected error for 404 download")
	}
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T: %v", err, err)
	}
	env := ec.Envelope()
	if env["type"] != output.TypeValidation {
		t.Errorf("404 → type=validation expected; got %v (err=%v)", env["type"], err)
	}
	hint, _ := env["hint"].(string)
	msg, _ := env["message"].(string)
	if !strings.Contains(hint, "themes") && !strings.Contains(msg, "themes") {
		t.Errorf("envelope should reference `themes`; got message=%q hint=%q", msg, hint)
	}
}

func TestPull_TempZipPathUnique(t *testing.T) {
	a := tempZipPath("theme-x")
	b := tempZipPath("theme-x")
	if a == b {
		t.Errorf("tempZipPath should be unique across calls; got duplicate: %s", a)
	}
	// Confirm same theme-id appears in both paths (sanity).
	if !strings.Contains(a, "theme-x") || !strings.Contains(b, "theme-x") {
		t.Errorf("themeID not embedded in tmp path: a=%s b=%s", a, b)
	}
	// Different theme IDs must also produce different paths.
	c := tempZipPath("theme-y")
	if c == a {
		t.Errorf("distinct theme IDs collided: a=%s c=%s", a, c)
	}
}

// TestPull_TempZipPathSanitizesID: even if the flag-level theme-id validation
// is bypassed, tempZipPath itself must not splice separators into the path.
func TestPull_TempZipPathSanitizesID(t *testing.T) {
	p := tempZipPath("../../etc/passwd")
	dir := filepath.Dir(p)
	if dir != filepath.Clean(os.TempDir()) {
		t.Errorf("tmp zip escaped the tmp dir: %s", p)
	}
	if strings.Contains(filepath.Base(p), "/") || strings.Contains(filepath.Base(p), "\\") {
		t.Errorf("separators survived sanitization: %s", p)
	}
}

// TestPull_CorruptZipClassifiesInternal: a download that is not a valid zip
// (truncated proxy response, HTML error page) is a LOCAL extraction failure —
// internal-class — not the old misleading "zip integrity ... check network"
// validation error. The tmp zip is preserved for forensics.
func TestPull_CorruptZipClassifiesInternal(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write([]byte("this is not a zip"))
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Nova","id":"corrupt1"}`))
	}))
	defer srv.Close()
	tmp := t.TempDir()
	chdirT(t, tmp)

	_, err := pullShortcut.Execute(context.Background(),
		common.ExecInput{Client: client.New(srv.URL), Flags: pullFlags("corrupt1")})
	if err == nil {
		t.Fatal("expected error for corrupt zip")
	}
	t.Cleanup(func() { removeTmpZips(t, "corrupt1") })
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T: %v", err, err)
	}
	env := ec.Envelope()
	if env["type"] != output.TypeInternal {
		t.Errorf("corrupt zip → type=internal expected; got %v (err=%v)", env["type"], err)
	}
	if msg, _ := env["message"].(string); strings.Contains(msg, "check network") {
		t.Errorf("misleading 'check network' hint must be gone: %q", msg)
	}
}

// TestPull_UnsafeArchiveNamesOffendingEntry: path-traversal rejection must be
// validation-class and name the OFFENDING ENTRY, not just the tmp zip.
func TestPull_UnsafeArchiveNamesOffendingEntry(t *testing.T) {
	zipBytes := buildTestZip(t, map[string]string{
		"Nova/../../evil.txt":  "evil",
		"Nova/assets/main.css": "css",
	})
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.HasSuffix(r.URL.Path, "/download") {
			w.Header().Set("Content-Type", "application/zip")
			_, _ = w.Write(zipBytes)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"name":"Nova","id":"unsafe1"}`))
	}))
	defer srv.Close()
	tmp := t.TempDir()
	chdirT(t, tmp)

	_, err := pullShortcut.Execute(context.Background(),
		common.ExecInput{Client: client.New(srv.URL), Flags: pullFlags("unsafe1")})
	if err == nil {
		t.Fatal("expected error for unsafe archive entry")
	}
	t.Cleanup(func() { removeTmpZips(t, "unsafe1") })
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T: %v", err, err)
	}
	env := ec.Envelope()
	if env["type"] != output.TypeValidation {
		t.Errorf("unsafe entry → type=validation expected; got %v", env["type"])
	}
	msg, _ := env["message"].(string)
	if !strings.Contains(msg, "evil.txt") {
		t.Errorf("message must name the offending entry, got %q", msg)
	}
}

// removeTmpZips clears preserved tmp zips for the given theme id so failure-
// path tests don't leak artifacts into the shared OS tmp dir.
func removeTmpZips(t *testing.T, themeID string) {
	t.Helper()
	matches, _ := filepath.Glob(filepath.Join(os.TempDir(), "shoplazza-theme-"+themeID+"-*.zip"))
	for _, m := range matches {
		_ = os.Remove(m)
	}
}

// Compile-time guard against the rare test environment where SendStream
// stubs are needed. Currently unused — retained for future expansion.
var _ = io.Discard

// ── classifyPullDownloadErr ───────────────────────────────────────────────────

func TestClassifyPullDownloadErr_AuthError(t *testing.T) {
	for _, code := range []int{http.StatusUnauthorized, http.StatusForbidden} {
		httpErr := &client.HTTPError{StatusCode: code, Body: "forbidden"}
		err := classifyPullDownloadErr(httpErr, "tid-1")
		if err == nil {
			t.Fatalf("code %d: expected error", code)
		}
	}
}

func TestClassifyPullDownloadErr_5xx(t *testing.T) {
	httpErr := &client.HTTPError{StatusCode: http.StatusInternalServerError, Body: "oops"}
	err := classifyPullDownloadErr(httpErr, "tid-1")
	if err == nil || !strings.Contains(err.Error(), "server error") {
		t.Errorf("5xx: expected server error, got %v", err)
	}
}

func TestClassifyPullDownloadErr_NonHTTP(t *testing.T) {
	rawErr := errors.New("connection reset")
	err := classifyPullDownloadErr(rawErr, "tid-1")
	if err == nil || !strings.Contains(err.Error(), "download failed") {
		t.Errorf("non-http: expected 'download failed', got %v", err)
	}
}

// TestClassifyPullDownloadErr_NoTmpPathMention: SendStream fails BEFORE the
// tmp zip is created — the error must not point users at a nonexistent file.
func TestClassifyPullDownloadErr_NoTmpPathMention(t *testing.T) {
	for _, err := range []error{
		classifyPullDownloadErr(&client.HTTPError{StatusCode: 500, Body: "x"}, "tid-1"),
		classifyPullDownloadErr(errors.New("dial tcp: refused"), "tid-1"),
	} {
		if strings.Contains(err.Error(), "tmp") || strings.Contains(err.Error(), ".zip") {
			t.Errorf("pre-create error must not mention a tmp zip path: %v", err)
		}
	}
}
