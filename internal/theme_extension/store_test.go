package theme_extension

import (
	"archive/zip"
	"context"
	"encoding/json"
	"errors"
	"net"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func TestRegisterWritesExtensionIDAndDoesNotConnect(t *testing.T) {
	connectHit := false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_new"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			_, _ = w.Write([]byte(`{"task_id":"task_1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/task_1":
			_, _ = w.Write([]byte(`{"task_id":"task_1","state":1,"version_id":"ver_1"}`))
		case r.URL.Path == "/openapi/2025-06/theme-extensions/connection":
			connectHit = true
			_, _ = w.Write([]byte(`{"code":"Success"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	root := t.TempDir()
	if err := WriteConfig(root, Config{Name: "ext-x", Type: "theme", Subtype: "basic"}); err != nil {
		t.Fatal(err)
	}
	store := client.New(srv.URL)
	res, err := Register(context.Background(), store, root, "ext-x", "https://cdn/x.zip", "", "", time.Millisecond, 5)
	if err != nil {
		t.Fatalf("register: %v", err)
	}
	if connectHit {
		t.Fatal("Register must NOT connect")
	}
	if res.ExtensionID != "tex_new" {
		t.Fatalf("expected tex_new, got %q", res.ExtensionID)
	}
	cfg, _ := ReadConfig(root)
	if cfg.ExtensionID != "tex_new" {
		t.Fatalf("extension_id not written back: %+v", cfg)
	}
}

func TestRegisterHonorsVersionOnFreshExtension(t *testing.T) {
	var taskVersion, taskExts string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPut && r.URL.Path == "/openapi/2020-07/theme-extensions":
			_, _ = w.Write([]byte(`{"extension_id":"tex_new"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			taskVersion, _ = body["version"].(string)
			taskExts, _ = body["exts"].(string)
			_, _ = w.Write([]byte(`{"task_id":"t1"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/t1":
			_, _ = w.Write([]byte(`{"task_id":"t1","state":1,"version_id":"ver_x"}`))
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	root := t.TempDir()
	_ = WriteConfig(root, Config{Name: "ext-x", Type: "theme", Subtype: "basic"}) // no extension_id → create path
	if _, err := Register(context.Background(), client.New(srv.URL), root, "ext-x", "https://cdn/x.zip", "2.3.0", "release notes", time.Millisecond, 5); err != nil {
		t.Fatal(err)
	}
	if taskVersion != "2.3.0" {
		t.Fatalf("first-build --version dropped: version-task sent %q, want 2.3.0", taskVersion)
	}
	if taskExts != "release notes" {
		t.Fatalf("first-build --description dropped: version-task sent exts=%q, want %q", taskExts, "release notes")
	}
}

func TestZipThemeAppIncludesWrapper(t *testing.T) {
	root := t.TempDir()
	// project/theme-app/{blocks/x.liquid, assets-manifest.json}
	if err := os.MkdirAll(filepath.Join(root, "theme-app", "blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "theme-app", "blocks", "x.liquid"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "theme-app", "assets-manifest.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	zipPath, err := ZipThemeApp(root)
	if err != nil {
		t.Fatal(err)
	}
	names := zipEntryNames(t, zipPath)
	// The backend doc-parser requires the "theme-app/" wrapper prefix (v1 parity):
	// entries must be "theme-app/blocks/x.liquid", NOT a flattened "blocks/x.liquid".
	// A flat zip yields a doc-less version that fails `te release` with "version has
	// no doc". The manifest must sit at theme-app root: "theme-app/assets-manifest.json".
	if !contains(names, "theme-app/blocks/x.liquid") {
		t.Fatalf("expected theme-app/blocks/x.liquid (wrapped), got %v", names)
	}
	if !contains(names, "theme-app/assets-manifest.json") {
		t.Fatalf("expected theme-app/assets-manifest.json at the wrapper root, got %v", names)
	}
	if contains(names, "blocks/x.liquid") {
		t.Fatalf("zip must NOT contain a flattened (unwrapped) entry; got %v", names)
	}
}

// TestPushDevDoctreePollsTaskToCompletion: the full-tree dev push is async — the
// PATCH dev-doctree response carries {task_id}, which must be polled to completion
// at GET /version-tasks/{id} (v1 parity). It must not POST /version-tasks (that
// would mint a formal version = version explosion, the thing this avoids).
func TestPushDevDoctreePollsTaskToCompletion(t *testing.T) {
	hitDoctree, polled, postedVersionTask := false, false, false
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case r.Method == http.MethodPatch && r.URL.Path == "/openapi/2020-07/theme-extensions/tex_1/dev-doctree":
			hitDoctree = true
			_, _ = w.Write([]byte(`{"task_id":"dt_1"}`))
		case r.Method == http.MethodPost && r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks":
			postedVersionTask = true
			_, _ = w.Write([]byte(`{"task_id":"vt"}`))
		case r.URL.Path == "/openapi/2020-07/theme-extensions/version-tasks/dt_1":
			polled = true
			_, _ = w.Write([]byte(`{"task_id":"dt_1","state":1}`)) // done
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	store := client.New(srv.URL)
	if err := PushDevDoctree(context.Background(), store, "tex_1", "https://cdn/x.zip", time.Millisecond, 5); err != nil {
		t.Fatalf("push: %v", err)
	}
	if !hitDoctree {
		t.Fatal("expected dev-doctree PATCH")
	}
	if !polled {
		t.Fatal("expected the dev-doctree task to be polled at GET version-tasks/{id} (v1 parity)")
	}
	if postedVersionTask {
		t.Fatal("serve dev push must NOT POST /version-tasks (formal version = version explosion)")
	}
}

// TestPushDevDoctreeTaskFailureIsError: a failed dev-doctree task (state 2) must
// surface as an error, not a silent success — and as API-class (state 2 is a
// server-reported failure, not a CLI bug).
func TestPushDevDoctreeTaskFailureIsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/openapi/2020-07/theme-extensions/tex_1/dev-doctree":
			_, _ = w.Write([]byte(`{"task_id":"dt_1"}`))
		case "/openapi/2020-07/theme-extensions/version-tasks/dt_1":
			_, _ = w.Write([]byte(`{"task_id":"dt_1","state":2,"message":"boom"}`))
		default:
			_, _ = w.Write([]byte(`{}`))
		}
	}))
	defer srv.Close()
	err := PushDevDoctree(context.Background(), client.New(srv.URL), "tex_1", "https://cdn/x.zip", time.Millisecond, 5)
	if err == nil {
		t.Fatal("expected error when dev-doctree task state=2")
	}
	if err.Code != output.ExitAPI {
		t.Fatalf("state-2 failure is server-reported: exit %d, want %d (api)", err.Code, output.ExitAPI)
	}
}

// TestDevDocFileOps: incremental serve — create/update/delete a single file via
// the per-file dev-doc endpoint, matching v1's request shapes.
func TestDevDocFileOps(t *testing.T) {
	type capture struct{ method, ctype, location, content, qtype, qloc string }
	var got capture
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/2020-07/theme-extensions/tex_1/dev-doc" {
			got.method = r.Method
			if r.Method == http.MethodDelete {
				got.qtype = r.URL.Query().Get("type")
				got.qloc = r.URL.Query().Get("location")
			} else {
				var b map[string]any
				_ = json.NewDecoder(r.Body).Decode(&b)
				got.ctype, _ = b["type"].(string)
				got.location, _ = b["location"].(string)
				got.content, _ = b["content"].(string)
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	store := client.New(srv.URL)
	if err := CreateDevDocFile(context.Background(), store, "tex_1", "blocks", "foo.liquid", "<x>"); err != nil {
		t.Fatalf("create: %v", err)
	}
	if got.method != http.MethodPost || got.ctype != "blocks" || got.location != "foo.liquid" || got.content != "<x>" {
		t.Fatalf("create shape: %+v", got)
	}
	if err := UpdateDevDocFile(context.Background(), store, "tex_1", "snippets", "bar.liquid", "<y>"); err != nil {
		t.Fatalf("update: %v", err)
	}
	if got.method != http.MethodPatch || got.ctype != "snippets" || got.location != "bar.liquid" || got.content != "<y>" {
		t.Fatalf("update shape: %+v", got)
	}
	if err := DeleteDevDocFile(context.Background(), store, "tex_1", "assets", "app.css"); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if got.method != http.MethodDelete || got.qtype != "assets" || got.qloc != "app.css" {
		t.Fatalf("delete shape: %+v", got)
	}
}

// TestUpsertDevDocFile_FallsBackToCreateOnMissing: an upsert PATCHes first; if
// the per-file doc isn't there yet (backend "extension doc not found"), it must
// fall back to POST-create — so a watcher-mislabeled "update" of a not-yet-
// created file still lands instead of failing with "doc not found".
func TestUpsertDevDocFile_FallsBackToCreateOnMissing(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/2020-07/theme-extensions/tex_1/dev-doc" {
			methods = append(methods, r.Method)
			if r.Method == http.MethodPatch {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"message":"extension doc not found"}`))
				return
			}
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	store := client.New(srv.URL)

	if ex := UpsertDevDocFile(context.Background(), store, "tex_1", "assets", "basic.css", "/* x */"); ex != nil {
		t.Fatalf("upsert should succeed via create-fallback; got %v", ex)
	}
	if len(methods) != 2 || methods[0] != http.MethodPatch || methods[1] != http.MethodPost {
		t.Fatalf("expected PATCH then POST (create-on-missing); got %v", methods)
	}
}

// TestUpsertDevDocFile_PatchOnlyWhenPresent: when the doc already exists, the
// upsert is a single PATCH — no spurious POST.
func TestUpsertDevDocFile_PatchOnlyWhenPresent(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/2020-07/theme-extensions/tex_1/dev-doc" {
			methods = append(methods, r.Method)
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	store := client.New(srv.URL)

	if ex := UpsertDevDocFile(context.Background(), store, "tex_1", "assets", "basic.css", "/* x */"); ex != nil {
		t.Fatalf("upsert: %v", ex)
	}
	if len(methods) != 1 || methods[0] != http.MethodPatch {
		t.Fatalf("expected a single PATCH when the doc exists; got %v", methods)
	}
}

// TestUpsertDevDocFile_500NotFoundIsNotMissing: the "not found" substring only
// means "create the doc" on a 4xx. A PATCH 500 whose body happens to say "store
// not found" is a server fault — it must not trigger a spurious POST whose 200
// would turn the whole upsert into reported success.
func TestUpsertDevDocFile_500NotFoundIsNotMissing(t *testing.T) {
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		methods = append(methods, r.Method)
		if r.Method == http.MethodPatch {
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"store not found"}`))
			return
		}
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()

	ex := UpsertDevDocFile(context.Background(), client.New(srv.URL), "tex_1", "assets", "basic.css", "/* x */")
	if ex == nil {
		t.Fatal("a 500 'store not found' must surface as an error, not silent create-success")
	}
	if ex.Code != output.ExitAPI {
		t.Fatalf("exit %d, want %d (api)", ex.Code, output.ExitAPI)
	}
	if len(methods) != 1 || methods[0] != http.MethodPatch {
		t.Fatalf("a 5xx must not trigger the create fallback; got %v", methods)
	}
}

// TestDevDocMissingRequires4xx pins the classification table directly.
func TestDevDocMissingRequires4xx(t *testing.T) {
	cases := []struct {
		status int
		body   string
		want   bool
	}{
		{404, `{}`, true},
		{400, `{"message":"extension doc not found"}`, true},
		{422, `{"message":"Not Found"}`, true},
		{500, `{"message":"store not found"}`, false},
		{502, `not found`, false},
		{400, `{"message":"bad request"}`, false},
	}
	for _, c := range cases {
		got := devDocMissing(&client.HTTPError{StatusCode: c.status, Body: c.body})
		if got != c.want {
			t.Errorf("devDocMissing(%d, %q) = %v, want %v", c.status, c.body, got, c.want)
		}
	}
}

// TestDevDocTarget: path→(type,location) replicates v1 getFileInfo — type is the
// IMMEDIATE parent dir, location is the base name (nesting collapses to the
// nearest parent, exactly as v1 does).
func TestDevDocTarget(t *testing.T) {
	cases := []struct{ rel, wantType, wantLoc string }{
		{"blocks/foo.liquid", "blocks", "foo.liquid"},
		{"assets/app.css", "assets", "app.css"},
		{"snippets/sub/x.liquid", "sub", "x.liquid"},
	}
	for _, c := range cases {
		gt, gl := DevDocTarget(c.rel)
		if gt != c.wantType || gl != c.wantLoc {
			t.Errorf("DevDocTarget(%q) = (%q,%q), want (%q,%q)", c.rel, gt, gl, c.wantType, c.wantLoc)
		}
	}
}

// zipEntryNames opens a zip archive and returns all entry names.
func zipEntryNames(t *testing.T, zipPath string) []string {
	t.Helper()
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		t.Fatalf("open zip %s: %v", zipPath, err)
	}
	defer r.Close()
	var names []string
	for _, f := range r.File {
		names = append(names, f.Name)
	}
	return names
}

func TestListVersionsDrillsArray(t *testing.T) {
	// double-data envelope (v1's suspected GET shape) — the reason map-nav exists.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"data":[{"version_id":"v1","version":"1.0.0","published":true}]}}`))
	}))
	defer srv.Close()
	vers, err := ListVersions(context.Background(), client.New(srv.URL), "tex_1")
	if err != nil {
		t.Fatal(err)
	}
	if len(vers) != 1 || vers[0]["version"] != "1.0.0" {
		t.Fatalf("unexpected versions: %v", vers)
	}
}

func TestListExtensionsDrillsArray(t *testing.T) {
	// single-data envelope
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":[{"extension_id":"tex_1","title":"My Ext","created_at":"2026-01-01"}]}`))
	}))
	defer srv.Close()
	exts, err := ListExtensions(context.Background(), client.New(srv.URL))
	if err != nil {
		t.Fatal(err)
	}
	if len(exts) != 1 || exts[0]["extension_id"] != "tex_1" {
		t.Fatalf("unexpected: %v", exts)
	}
}

func TestPublishPostsEnableBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer srv.Close()
	err := Publish(context.Background(), client.New(srv.URL), StorePublicationsPath, "tex_1", "ver_1")
	if err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != StorePublicationsPath {
		t.Fatalf("path %q", gotPath)
	}
	if gotBody["type"] != "enable" || gotBody["extension_id"] != "tex_1" || gotBody["version_id"] != "ver_1" {
		t.Fatalf("body %v", gotBody)
	}
}

// TestPublishSurfacesBusinessFailureIn2xx: the publications endpoint reports
// failures inside 2xx bodies (e.g. 200 {"code":"4001","message":"version not
// found"}) — Publish must not swallow them.
func TestPublishSurfacesBusinessFailureIn2xx(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"code":"4001","message":"version not found"}`))
	}))
	defer srv.Close()
	err := Publish(context.Background(), client.New(srv.URL), StorePublicationsPath, "tex_1", "ver_missing")
	if err == nil {
		t.Fatal("expected error for a 2xx body carrying a failure code")
	}
	if err.Code != output.ExitAPI {
		t.Fatalf("business failure is API-class: exit %d, want %d", err.Code, output.ExitAPI)
	}
	if err.Detail == nil || !strings.Contains(err.Detail.Message, "version not found") {
		t.Fatalf("error should carry the server message, got %+v", err.Detail)
	}
}

// TestPublishAcceptsSuccessShapes: both historical success markers and the
// unwrapped-envelope (empty code) shape stay success.
func TestPublishAcceptsSuccessShapes(t *testing.T) {
	for _, body := range []string{`{"code":"Success"}`, `{"code":200}`, `{}`, `{"code":"Success","data":{}}`} {
		srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
			_, _ = w.Write([]byte(body))
		}))
		if err := Publish(context.Background(), client.New(srv.URL), StorePublicationsPath, "tex_1", "ver_1"); err != nil {
			t.Errorf("body %s should be success, got %v", body, err)
		}
		srv.Close()
	}
}

// TestApiOrInternalTE_Classification: HTTP → api with the failing endpoint
// attached; wire failure → network; anything else → internal.
func TestApiOrInternalTE_Classification(t *testing.T) {
	he := &client.HTTPError{StatusCode: 422, Body: `{"message":"bad"}`, Method: "PATCH", Path: "/openapi/x"}
	got := apiOrInternalTE(he)
	if got.Code != output.ExitAPI {
		t.Fatalf("HTTPError: exit %d, want api", got.Code)
	}
	if got.Detail == nil || got.Detail.Detail == nil || got.Detail.Detail.Method != "PATCH" || got.Detail.Detail.Path != "/openapi/x" {
		t.Fatalf("HTTPError should name the endpoint, got %+v", got.Detail)
	}
	if got := apiOrInternalTE(&client.HTTPError{StatusCode: 403, Body: `{}`}); got.Code != output.ExitAuth {
		t.Fatalf("403: exit %d, want auth", got.Code)
	}
	dial := &url.Error{Op: "Patch", URL: "https://s/x", Err: &net.OpError{Op: "dial", Err: errors.New("connection refused")}}
	if got := apiOrInternalTE(dial); got.Code != output.ExitNetwork {
		t.Fatalf("refused dial: exit %d, want network", got.Code)
	}
	if got := apiOrInternalTE(errors.New("boom")); got.Code != output.ExitInternal {
		t.Fatalf("generic: exit %d, want internal", got.Code)
	}
}

// TestZipThemeAppMissingDirIsSentinel: callers must be able to tell the
// user-fixable "no theme-app/" apart from environment failures via errors.Is.
func TestZipThemeAppMissingDirIsSentinel(t *testing.T) {
	if _, err := ZipThemeApp(t.TempDir()); !errors.Is(err, ErrThemeAppMissing) {
		t.Fatalf("missing dir: expected ErrThemeAppMissing, got %v", err)
	}
	// exists but is a regular file → same sentinel
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "theme-app"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, err := ZipThemeApp(root); !errors.Is(err, ErrThemeAppMissing) {
		t.Fatalf("non-dir: expected ErrThemeAppMissing, got %v", err)
	}
}

func TestPublicationsPathsAreDistinct(t *testing.T) {
	if StorePublicationsPath == PartnerPublicationsPath {
		t.Fatal("StorePublicationsPath must differ from PartnerPublicationsPath (guard)")
	}
}

// TestPublishIsPathFaithful proves Publish posts to exactly the path it is given
// — so `te release` (PartnerPublicationsPath) and `te deploy` (StorePublicationsPath)
// cannot be conflated by Publish itself.
func TestPublishIsPathFaithful(t *testing.T) {
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer srv.Close()
	if err := Publish(context.Background(), client.New(srv.URL), PartnerPublicationsPath, "tex_1", "ver_1"); err != nil {
		t.Fatalf("publish: %v", err)
	}
	if gotPath != PartnerPublicationsPath {
		t.Fatalf("expected partner path %q, got %q", PartnerPublicationsPath, gotPath)
	}
}

func contains(s []string, target string) bool {
	for _, v := range s {
		if v == target {
			return true
		}
	}
	return false
}
