package checkout_test

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	checkout "github.com/Shoplazza/shoplazza-cli/v2/cmd/checkout"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// skipIfDirWritable skips the test when a write into dir still succeeds despite a
// prior chmod 0o555 — i.e. chmod-based denial isn't enforced here (running as
// root, or a temp dir on a filesystem that ignores directory permissions). The
// test exercises a write-failure path that can't be induced in that case, so
// skipping is correct; asserting failure would be a false negative.
func skipIfDirWritable(t *testing.T, dir string) {
	t.Helper()
	probe := filepath.Join(dir, ".write-probe")
	if f, err := os.Create(probe); err == nil {
		_ = f.Close()
		_ = os.Remove(probe)
		t.Skipf("%s is writable despite chmod 0o555 (root or a permissive filesystem); cannot exercise the write-failure path", dir)
	}
}

// makeExtProject creates extensions/<id>/extension.json (extensionId = extensionID arg,
// empty = first push) plus a built artifact, and returns the project root + extension dir.
func makeExtProject(t *testing.T, extID string, extensionID string) (root, extDir string) {
	t.Helper()
	root = t.TempDir()
	extDir = filepath.Join(root, "extensions", extID)
	_ = os.MkdirAll(filepath.Join(extDir, "src"), 0o755)
	cfg := map[string]any{
		"version": "1.3", "templateName": "checkout", "themeName": "",
		"extensionId": extensionID, "extensionName": "Demo", "extensionDescription": "d",
	}
	b, _ := json.MarshalIndent(cfg, "", "  ")
	_ = os.WriteFile(filepath.Join(extDir, "extension.json"), b, 0o644)
	_ = os.MkdirAll(filepath.Join(root, "dist"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "dist", extID+".abc123.js"), []byte("built"), 0o644)
	return root, extDir
}

func newPushFactory(t *testing.T, srvURL string) *cmdutil.Factory {
	f, _ := tempCheckoutFactory(t, srvURL)
	return f
}

func TestRunPush_FirstPush_CreatesAndWritesBackID(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "") // empty extensionId → create
	var createBody map[string]any
	var previewBody map[string]any
	var ossURL string

	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/",
			"policy": "P", "access_id": "AK", "sign": "SG",
		})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&createBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER1", "name": "Demo"}},
		})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&previewBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"checkout_url": "/c/x"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	f := newPushFactory(t, srv.URL)
	out := &bytes.Buffer{}
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(out)
	cmd.SetContext(context.Background())

	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "test-store.myshoplaza.com"); err != nil {
		t.Fatalf("runPush: %v", err)
	}

	inner := createBody["extension"].(map[string]any)
	if v, ok := inner["scope"]; !ok || v != "" {
		t.Errorf("create body must carry scope:'', got %v ok=%v", v, ok)
	}
	if inner["resource_url"] != "https://read.example/chick-extension/demo.abc123.js" {
		t.Errorf("resource_url = %v", inner["resource_url"])
	}
	if !strings.Contains(inner["extends_fields"].(string), `"version"`) {
		t.Errorf("extends_fields must be the stringified extension.json, got %v", inner["extends_fields"])
	}
	raw, _ := os.ReadFile(filepath.Join(extDir, "extension.json"))
	var cfg map[string]any
	_ = json.Unmarshal(raw, &cfg)
	if cfg["extensionId"] != "SRV1" {
		t.Fatalf("first push must write back extensionId=SRV1, got %v", cfg["extensionId"])
	}
	pinner := previewBody["extension"].(map[string]any)
	if pinner["extension_id"] != "SRV1" || pinner["id"] != "VER1" {
		t.Errorf("preview should use SRV1/VER1, got %v", previewBody)
	}
}

// TestRunPush_RealStatusEnvelope_SurfacesID locks the REAL server shape:
// create replies with status/message success (NOT code:"Success"/ok), so the
// client does NOT unwrap and the extension sits under .data.extension.
// Regression guard for the bug where push returned an empty extension_id.
func TestRunPush_RealStatusEnvelope_SurfacesID(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "") // first push
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":    map[string]any{"extension": map[string]any{"extension_id": "SRV9", "id": "VER9", "name": "Demo"}},
			"errors":  []any{},
			"message": "success",
			"status":  0,
		})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"checkout_url": "/c/x"}, "message": "success", "status": 0})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	out := &bytes.Buffer{}
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(out)
	cmd.SetContext(context.Background())
	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com"); err != nil {
		t.Fatalf("runPush (real envelope): %v", err)
	}
	var env map[string]any
	_ = json.Unmarshal(out.Bytes(), &env)
	if env["extension_id"] != "SRV9" || env["version_id"] != "VER9" {
		t.Fatalf("push must surface extension_id/version_id from the real envelope, got %v", env)
	}
	raw, _ := os.ReadFile(filepath.Join(extDir, "extension.json"))
	var cfg map[string]any
	_ = json.Unmarshal(raw, &cfg)
	if cfg["extensionId"] != "SRV9" {
		t.Errorf("first push must write back extensionId=SRV9, got %v", cfg["extensionId"])
	}
}

func TestRunPush_SubsequentPush_Commits(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "SRV1") // has extensionId → commit
	var hitCommit bool
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		hitCommit = true
		var body map[string]any
		_ = json.NewDecoder(r.Body).Decode(&body)
		if body["extension"].(map[string]any)["extension_id"] != "SRV1" {
			t.Error("commit must carry the existing extension_id")
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER2"}}})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"checkout_url": "/c/x"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(io.Discard)
	cmd.SetContext(context.Background())
	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com"); err != nil {
		t.Fatalf("runPush: %v", err)
	}
	if !hitCommit {
		t.Error("extension with existing extensionId must hit /commit")
	}
}

// TestRunPush_Commit_RealEnvelope_SurfacesIDAndPreview guards the COMMIT path:
// the real server envelope is {data:{extension:{extension_id,id}}, message,
// status} (no code:"Success"/ok, so the client does NOT unwrap). push must parse
// extension_id/version_id from it and produce a preview_url — the prior commit
// test used io.Discard + a code:"Success" mock and never checked this, so a
// "commit push has no preview_url" regression went unnoticed.
func TestRunPush_Commit_RealEnvelope_SurfacesIDAndPreview(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "SRV1") // has extensionId → commit
	var previewHit bool
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// REAL commit envelope (single data.extension, status/message success).
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":    map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER2", "name": "Demo"}},
			"errors":  []any{},
			"message": "success",
			"status":  0,
		})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		previewHit = true
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"checkout_url": "/c/x"}, "message": "success", "status": 0})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	out := &bytes.Buffer{}
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(out)
	cmd.SetContext(context.Background())
	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com"); err != nil {
		t.Fatalf("runPush (commit, real envelope): %v", err)
	}
	var env map[string]any
	_ = json.Unmarshal(out.Bytes(), &env)
	if env["extension_id"] != "SRV1" || env["version_id"] != "VER2" {
		t.Fatalf("commit must surface extension_id/version_id from the real envelope, got %v", env)
	}
	if !previewHit || env["preview_url"] == "" {
		t.Fatalf("commit must produce a preview_url, got preview_url=%v previewHit=%v", env["preview_url"], previewHit)
	}
}

// TestRunPush_Commit_InvalidVersion_Errors locks the fix for the 200-OK failure
// envelope: a commit that returns {"message":"INVALID_VERSION","status":3} must
// surface an error, not report ok:true with empty extension_id/version_id.
func TestRunPush_Commit_InvalidVersion_Errors(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "SRV1") // has extensionId → commit
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		// Real failure envelope: status != 0, no data (observed live).
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "INVALID_VERSION", "status": 3})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com")
	if err == nil {
		t.Fatal("expected an error when the server returns INVALID_VERSION (status:3), got nil")
	}
	// message is concise ("version ... already exists"); the actionable advice
	// (--version / bump) lives in the envelope hint, not the message.
	if !strings.Contains(err.Error(), "INVALID_VERSION") {
		t.Errorf("message should surface the raw server code; got: %v", err)
	}
	if strings.Contains(err.Error(), "--version") {
		t.Errorf("the --version suggestion belongs in the hint, not the message; got: %v", err)
	}
}

// TestRunPush_VersionFlag_WritesExtensionJSON: --version writes the new version
// into extension.json before pushing (the optional auto-bump).
func TestRunPush_VersionFlag_WritesExtensionJSON(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "SRV1") // commit path
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER2"}}, "message": "success", "status": 0})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"checkout_url": "/c/x"}, "message": "success", "status": 0})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.Flags().String("version", "", "")
	_ = cmd.Flags().Set("version", "9.9.9")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com"); err != nil {
		t.Fatalf("runPush with --version: %v", err)
	}
	b, _ := os.ReadFile(filepath.Join(extDir, "extension.json"))
	var cfg map[string]any
	_ = json.Unmarshal(b, &cfg)
	if cfg["version"] != "9.9.9" {
		t.Fatalf("--version should write version=9.9.9 into extension.json, got %v", cfg["version"])
	}
}

// TestRunPush_WritebackFailureCarriesIDs: when the server create succeeded but
// the extension.json writeback fails, the error must carry the new ids (and a
// recovery hint) — otherwise the user can't record them and the next push
// would re-create a duplicate extension.
func TestRunPush_WritebackFailureCarriesIDs(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "") // first push → create + writeback
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":    map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER1", "name": "Demo"}},
			"message": "success", "status": 0,
		})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	// Make the writeback fail: the atomic write creates a temp file in extDir.
	if err := os.Chmod(extDir, 0o555); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(extDir, 0o755) })
	skipIfDirWritable(t, extDir)

	f := newPushFactory(t, srv.URL)
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	cmd.SetOut(&bytes.Buffer{})
	cmd.SetContext(context.Background())
	err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected writeback failure error, got %v", err)
	}
	if !strings.Contains(ee.Detail.Message, "SRV1") || !strings.Contains(ee.Detail.Message, "VER1") {
		t.Fatalf("error must carry the new extension_id/version_id, got %q", ee.Detail.Message)
	}
	if !strings.Contains(ee.Detail.Hint, "extensionId") {
		t.Fatalf("hint must tell the user to record the extensionId manually, got %q", ee.Detail.Hint)
	}
}

// TestPush_PathEscapingNameRejected: --name must be a plain directory name —
// a path-escaping value is rejected by the validPlainName guard, not by a
// stat miss (which an existing outside path would pass).
func TestPush_PathEscapingNameRejected(t *testing.T) {
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "push", "--name", "../../x")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("path-escaping --name must be type=validation, got %v", err)
	}
	if !strings.Contains(ee.Detail.Message, "plain name") {
		t.Errorf("must be rejected by the plain-name guard, got %q", ee.Detail.Message)
	}
}

// TestPush_DryRunFlagHidden: push registers --dry-run via the shared API flags
// but rejects it at runtime, so the flag must not be advertised in help.
func TestPush_DryRunFlagHidden(t *testing.T) {
	f, _ := tempCheckoutFactory(t, "http://unused")
	root := checkout.NewCmdCheckout(f)
	push, _, err := root.Find([]string{"push"})
	if err != nil {
		t.Fatal(err)
	}
	fl := push.Flags().Lookup("dry-run")
	if fl == nil || !fl.Hidden {
		t.Fatal("push --dry-run must stay registered (runtime backstop) but be hidden from help")
	}
}

func TestRunPush_PreviewFailureIsNonFatal(t *testing.T) {
	root, extDir := makeExtProject(t, "demo", "")
	var ossURL string
	mux := http.NewServeMux()
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"write_host": ossURL, "read_host": "https://r/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(200) })
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extension": map[string]any{"extension_id": "SRV1", "id": "VER1"}}})
	})
	mux.HandleFunc("/openapi/checkout_extensions/preview", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError) // preview blows up
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"
	f := newPushFactory(t, srv.URL)
	cmd := checkout.NewCmdCheckout(f)
	cmd.PersistentFlags().String("format", "json", "")
	out := &bytes.Buffer{}
	cmd.SetOut(out)
	cmd.SetContext(context.Background())
	if err := checkout.RunPushForTest(context.Background(), cmd, f, extDir, filepath.Join(root, "dist", "demo.abc123.js"), "s.com"); err != nil {
		t.Fatalf("preview failure must NOT fail push, got: %v", err)
	}
	raw, _ := os.ReadFile(filepath.Join(extDir, "extension.json"))
	var cfg map[string]any
	_ = json.Unmarshal(raw, &cfg)
	if cfg["extensionId"] != "SRV1" {
		t.Error("writeback should still happen even if preview fails")
	}
}
