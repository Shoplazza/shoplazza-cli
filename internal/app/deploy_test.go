package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/BurntSushi/toml"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
)

// captureDeployPayloadExt captures the first extension from the /deploy request
// body's extensions array. The deploy body is enveloped as {"app": {...}}.
func captureDeployPayloadExt(t *testing.T, body []byte) map[string]any {
	t.Helper()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("captureDeployPayloadExt: unmarshal envelope: %v", err)
	}
	app, _ := env["app"].(map[string]any)
	if app == nil {
		t.Fatalf("captureDeployPayloadExt: no 'app' key in body %s", body)
	}
	exts, _ := app["extensions"].([]any)
	if len(exts) == 0 {
		t.Fatalf("captureDeployPayloadExt: no extensions in deploy body")
	}
	ext, _ := exts[0].(map[string]any)
	if ext == nil {
		t.Fatalf("captureDeployPayloadExt: extensions[0] is not a map")
	}
	return ext
}

func TestDeploy_CheckoutHappyPath(t *testing.T) {
	mux := http.NewServeMux()
	// Dashboard: no existing remote extensions -> local is new (create).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	// OSS sign
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// GenerateVersion -> app_version "v-gen-1", no per-extension versions (local is an add -> create at 1.0.0).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	// checkout create
	var createBody []byte
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		createBody, _ = io.ReadAll(r.Body)
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e1","id":"v1"}}}`))
	})
	// Dashboard deploy: capture body to assert extension_version + extension_version_id.
	var mu sync.Mutex
	var deployBody []byte
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		deployBody = b
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	dir := t.TempDir()
	d := NewDashboard(client.New(srv.URL), "ptok")
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")

	deps := DeployDeps{
		Dashboard: d, Store: store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	// Deploy returns the generated app_version (v1 parity), not the deploy response.
	if res.Version != "v-gen-1" {
		t.Fatalf("version = %q, want v-gen-1", res.Version)
	}
	if len(res.Extensions) != 1 || res.Extensions[0].ExtensionID != "e1" || res.Extensions[0].VersionID != "v1" {
		t.Fatalf("extensions = %+v", res.Extensions)
	}
	// An add (no extension_id) uses version "1.0.0" on the checkout create body.
	var createEnv map[string]any
	if err := json.Unmarshal(createBody, &createEnv); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	createExt, _ := createEnv["extension"].(map[string]any)
	if createExt == nil || createExt["version"] != "1.0.0" {
		t.Fatalf("checkout create version = %v, want 1.0.0; body = %s", createExt, createBody)
	}
	// Verify the deploy payload: app.version == generated app_version, and each
	// extension carries extension_version + extension_version_id (v1 parity).
	mu.Lock()
	b := deployBody
	mu.Unlock()
	var depEnv map[string]any
	if err := json.Unmarshal(b, &depEnv); err != nil {
		t.Fatalf("unmarshal deploy body: %v", err)
	}
	depApp, _ := depEnv["app"].(map[string]any)
	if depApp == nil || depApp["version"] != "v-gen-1" {
		t.Fatalf("deploy payload app.version = %v, want v-gen-1", depApp["version"])
	}
	ext := captureDeployPayloadExt(t, b)
	if ext["extension_version"] == "" || ext["extension_version"] == nil {
		t.Fatalf("deploy payload extension_version is empty; ext = %v", ext)
	}
	if ext["extension_version_id"] == "" || ext["extension_version_id"] == nil {
		t.Fatalf("deploy payload extension_version_id is empty; ext = %v", ext)
	}
}

// TestDeploy_FirstCreateWritesBackIDToExtensionToml: after a
// create-path upsert returns the new extension_id, deploy must persist it into
// extensions/<dir>/shoplazza.extension.toml (v1 deploy.js:156 / dev.js:154 did),
// so the next dev/deploy matches by id instead of falling back to name matching
// or re-creating the extension.
func TestDeploy_FirstCreateWritesBackIDToExtensionToml(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e-new-77","id":"v1"}}}`))
	})
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	// Real project layout: <root>/extensions/co/shoplazza.extension.toml without
	// an id yet (first deploy), plus an unrelated key that must survive.
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "co")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	tomlPath := filepath.Join(extDir, "shoplazza.extension.toml")
	if err := os.WriteFile(tomlPath,
		[]byte("name = \"co\"\ntype = \"checkout\"\nversion = \"1.0.0\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	d := NewDashboard(client.New(srv.URL), "ptok")
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")
	deps := DeployDeps{
		Dashboard: d, Store: store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1", ProjectRoot: root,
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(t.TempDir(), "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
	if _, ex := Deploy(context.Background(), deps); ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}

	var et struct {
		ID      string `toml:"id"`
		Name    string `toml:"name"`
		Type    string `toml:"type"`
		Version string `toml:"version"`
	}
	if _, err := toml.DecodeFile(tomlPath, &et); err != nil {
		t.Fatalf("re-read extension toml: %v", err)
	}
	if et.ID != "e-new-77" {
		t.Fatalf("extension toml id = %q, want server-issued e-new-77 (writeback)", et.ID)
	}
	if et.Name != "co" || et.Type != "checkout" || et.Version != "1.0.0" {
		t.Fatalf("other toml keys must be preserved, got %+v", et)
	}
}

// TestDeploy_CheckoutIdMatchUpdate proves the id-match update path:
// a local toml id "ext1" matches a remote extension_id "ext1" via Diff's Pass 1,
// the per-extension version comes from generateVersion's newExtensionVersions
// ("2.0.0"), the checkout COMMIT (not create) endpoint is hit with that version,
// and the app-level version is the generated app_version ("v-gen-1").
func TestDeploy_CheckoutIdMatchUpdate(t *testing.T) {
	mux := http.NewServeMux()
	// Remote has an existing checkout with extension_id "ext1".
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{
			map[string]any{"extension_id": "ext1", "extension_name": "co", "extension_type": "checkout"},
		}}})
	})
	// GenerateVersion: app_version "v-gen-1" and per-extension version "2.0.0" for ext1.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"app_version": "v-gen-1",
			"extensions":  []any{map[string]any{"extension_id": "ext1", "extension_version": "2.0.0"}},
		}})
	})
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// create MUST NOT be hit on the id-match update path.
	var createHit bool
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		createHit = true
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e1","id":"v1"}}}`))
	})
	// commit IS the expected path; capture its body.
	var mu sync.Mutex
	var commitBody []byte
	var commitHit bool
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		commitBody = b
		commitHit = true
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"ext1","id":"vid2"}}}`))
	})
	var deployBody []byte
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		deployBody = b
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "ignored", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	dir := t.TempDir()
	d := NewDashboard(client.New(srv.URL), "ptok")
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")

	deps := DeployDeps{
		Dashboard: d, Store: store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		// Local toml id "ext1" -> Diff Pass-1 id-match against the remote.
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0", ExtensionID: "ext1"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	mu.Lock()
	defer mu.Unlock()
	if createHit {
		t.Fatal("id-match update must NOT hit the checkout create endpoint")
	}
	if !commitHit {
		t.Fatal("id-match update must hit the checkout commit endpoint")
	}
	// Commit body: extension_id "ext1" and the GENERATED version "2.0.0".
	var commitEnv map[string]any
	if err := json.Unmarshal(commitBody, &commitEnv); err != nil {
		t.Fatalf("unmarshal commit body: %v", err)
	}
	cext, _ := commitEnv["extension"].(map[string]any)
	if cext == nil {
		t.Fatalf("commit body has no extension: %s", commitBody)
	}
	if cext["extension_id"] != "ext1" {
		t.Fatalf("commit extension_id = %v, want ext1", cext["extension_id"])
	}
	if cext["version"] != "2.0.0" {
		t.Fatalf("commit version = %v, want 2.0.0 (from generateVersion newExtensionVersions)", cext["version"])
	}
	// DeployResult.Version is the generated app_version, not the deploy response.
	if res.Version != "v-gen-1" {
		t.Fatalf("DeployResult.Version = %q, want v-gen-1", res.Version)
	}
	if len(res.Extensions) != 1 || res.Extensions[0].ExtensionID != "ext1" || res.Extensions[0].Version != "2.0.0" {
		t.Fatalf("extensions = %+v", res.Extensions)
	}
	// Deploy payload: app.version == generated app_version; extension carries the generated version.
	var depEnv map[string]any
	if err := json.Unmarshal(deployBody, &depEnv); err != nil {
		t.Fatalf("unmarshal deploy body: %v", err)
	}
	depApp, _ := depEnv["app"].(map[string]any)
	if depApp == nil || depApp["version"] != "v-gen-1" {
		t.Fatalf("deploy payload app.version = %v, want v-gen-1", depApp["version"])
	}
	ext := captureDeployPayloadExt(t, deployBody)
	if ext["extension_version"] != "2.0.0" {
		t.Fatalf("deploy payload extension_version = %v, want 2.0.0", ext["extension_version"])
	}
}

// TestDeploy_NoExtensions_DeploysAppVersion: a deploy with no local extensions
// is valid (v1 parity) — it still generates and deploys a new app version with
// an empty extensions list, rather than erroring.
func TestDeploy_NoExtensions_DeploysAppVersion(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "0.0.1", "extensions": []any{}}})
	})
	var mu sync.Mutex
	var deployBody []byte
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		deployBody = b
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "0.0.1"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"), Store: client.New(srv.URL), HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		Locals:        nil,
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) { return "", nil },
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("deploy with no extensions should succeed (v1 parity): %v", ex)
	}
	if res.Version != "0.0.1" {
		t.Fatalf("app version = %q, want 0.0.1", res.Version)
	}
	if len(res.Extensions) != 0 {
		t.Fatalf("expected no deployed extensions, got %d", len(res.Extensions))
	}
	// The /deploy payload carries an empty (non-null) extensions array.
	mu.Lock()
	body := deployBody
	mu.Unlock()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal deploy body: %v", err)
	}
	appPayload, _ := env["app"].(map[string]any)
	if appPayload == nil {
		t.Fatalf("no 'app' in deploy body: %s", body)
	}
	if exts, ok := appPayload["extensions"].([]any); !ok || len(exts) != 0 {
		t.Fatalf("deploy extensions should be an empty array, got %v", appPayload["extensions"])
	}
	if appPayload["version"] != "0.0.1" {
		t.Fatalf("deploy app version = %v, want 0.0.1", appPayload["version"])
	}
}

// TestDevReport_NoExtensions_RunsAnyway: dev with no local extensions is valid
// (v1 parity) — it still reports a dev session (the tunnel/OAuth install flow)
// with an empty extensions list, rather than erroring.
func TestDevReport_NoExtensions_RunsAnyway(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "0.0.1", "extensions": []any{}}})
	})
	var mu sync.Mutex
	var devBody []byte
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/dev", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		devBody = b
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"install_url": "https://store/install"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()

	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"), Store: client.New(srv.URL), HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		Locals:        nil,
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) { return "", nil },
	}
	res, ex := DevReport(context.Background(), deps, "https://pub", "/auth", "/auth/callback")
	if ex != nil {
		t.Fatalf("dev with no extensions should succeed (v1 parity): %v", ex)
	}
	if res.Version != "0.0.1" {
		t.Fatalf("app version = %q, want 0.0.1", res.Version)
	}
	if len(res.Extensions) != 0 {
		t.Fatalf("expected no extensions, got %d", len(res.Extensions))
	}
	if res.InstallURL != "https://store/install" || res.AppURL != "https://pub/auth" || res.RedirectURL != "https://pub/auth/callback" {
		t.Fatalf("dev URIs = install %q app %q redirect %q", res.InstallURL, res.AppURL, res.RedirectURL)
	}
	// The /dev payload carries an empty (non-null) extensions array.
	mu.Lock()
	body := devBody
	mu.Unlock()
	var env map[string]any
	if err := json.Unmarshal(body, &env); err != nil {
		t.Fatalf("unmarshal dev body: %v", err)
	}
	appPayload, _ := env["app"].(map[string]any)
	if exts, ok := appPayload["extensions"].([]any); !ok || len(exts) != 0 {
		t.Fatalf("dev extensions should be an empty array, got %v", appPayload["extensions"])
	}
}

func TestDeploy_ThemeLeg(t *testing.T) {
	var ossURL string
	var deployCalled bool
	// Combined Dashboard + Store(OSS sign/upload + 2020-07 theme) server.
	mux := http.NewServeMux()
	// Dashboard: no existing remote extensions -> local theme is an "add" (create path).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	// OSS sign (path is type-independent; up.Upload reuses the checkout_extensions sign endpoint).
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// GenerateVersion -> app_version "v-gen-1" (theme is an add -> create at 1.0.0).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	// 2020-07 theme endpoints: PUT theme-extensions -> {extension_id}, POST version-tasks -> {task_id},
	// GET version-tasks/{id} -> {state:1, version_id}.
	mux.HandleFunc("/openapi/2020-07/theme-extensions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"extension_id":"th1"}`))
	})
	mux.HandleFunc("/openapi/2020-07/theme-extensions/version-tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"t1"}`))
	})
	mux.HandleFunc("/openapi/2020-07/theme-extensions/version-tasks/t1", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"t1","state":1,"version_id":"tv1"}`))
	})
	// Dashboard deploy.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		deployCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	// Separate partner server for the 2025-06 connection (create path links the theme).
	var mu sync.Mutex
	var connBody map[string]any
	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var body map[string]any
		_ = json.Unmarshal(b, &body)
		mu.Lock()
		connBody = body
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	dir := t.TempDir()
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")

	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"),
		Store:     store, Partner: client.New(partnerSrv.URL),
		HTTPClient: srv.Client(),
		PartnerID:  "p1", ClientID: "cid_1",
		Locals: []LocalExt{{Dir: "th", Name: "th", Type: "theme", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "th.zip")
			_ = os.WriteFile(p, []byte("zipbytes"), 0o644)
			return p, nil
		},
		ThemePollInterval: time.Millisecond,
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	if !deployCalled {
		t.Fatal("Dashboard ExtensionDeploy was not called")
	}
	if len(res.Extensions) != 1 {
		t.Fatalf("extensions = %+v", res.Extensions)
	}
	e := res.Extensions[0]
	if e.Type != "theme" || e.ExtensionID != "th1" || e.VersionID != "tv1" {
		t.Fatalf("theme ext = %+v, want {theme, th1, tv1}", e)
	}
	if e.ResourceURL == "" {
		t.Fatalf("theme ext resource_url empty: %+v", e)
	}
	mu.Lock()
	defer mu.Unlock()
	if connBody == nil || connBody["extension_id"] != "th1" {
		t.Fatalf("partner connection not called for the theme (create path); body = %v", connBody)
	}
}

// TestDeploy_ThemeLeg_UpdatePathDoesNotConnect is the mirror of TestDeploy_ThemeLeg
// for the UPDATE path: a remote extension with extension_id "th1" matches the local
// theme's ExtensionID "th1" via Diff Pass-1, and GenerateVersion returns a non-empty
// per-extension version "2.0.0" for "th1". This makes upsertTheme's update=true, so
// ConnectTheme must NOT be called. Regression guard for the extraction of
// RegisterThemeExtension (connect only on create path).
func TestDeploy_ThemeLeg_UpdatePathDoesNotConnect(t *testing.T) {
	var ossURL string
	var deployCalled bool
	mux := http.NewServeMux()
	// Dashboard: existing remote theme with extension_id "th1" -> Diff Pass-1 id-match -> UPDATE path.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{
			map[string]any{"extension_id": "th1", "extension_name": "th", "extension_type": "theme"},
		}}})
	})
	// OSS sign (path is type-independent; up.Upload reuses the checkout_extensions sign endpoint).
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// GenerateVersion: app_version "v-gen-2" and per-extension version "2.0.0" for th1.
	// A non-empty extension_version for the matched id makes upsertTheme's update=true.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"app_version": "v-gen-2",
			"extensions":  []any{map[string]any{"extension_id": "th1", "extension_version": "2.0.0"}},
		}})
	})
	// 2020-07 theme endpoints: PUT returns the existing extension_id "th1",
	// POST version-tasks -> {task_id}, GET version-tasks/{id} -> {state:1, version_id}.
	mux.HandleFunc("/openapi/2020-07/theme-extensions", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"extension_id":"th1"}`))
	})
	mux.HandleFunc("/openapi/2020-07/theme-extensions/version-tasks", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"t2"}`))
	})
	mux.HandleFunc("/openapi/2020-07/theme-extensions/version-tasks/t2", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"task_id":"t2","state":1,"version_id":"tv2"}`))
	})
	// Dashboard deploy.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		deployCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "2.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	// Separate partner server that records whether the connection endpoint was hit.
	// Still wire it with the correct URL so that any regression calling ConnectTheme
	// is caught by connectHit=true (not a nil deref or network error).
	var mu sync.Mutex
	var connectHit bool
	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		connectHit = true
		mu.Unlock()
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"Success"}`))
	}))
	defer partnerSrv.Close()

	dir := t.TempDir()
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")

	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"),
		Store:     store, Partner: client.New(partnerSrv.URL),
		HTTPClient: srv.Client(),
		PartnerID:  "p1", ClientID: "cid_1",
		// Local theme ExtensionID "th1" matches the remote -> Diff Pass-1 id-match -> update path.
		// ExtensionVersion is set (non-empty) so that upsertTheme's update gate triggers.
		Locals: []LocalExt{{Dir: "th", Name: "th", Type: "theme", Version: "1.0.0", ExtensionID: "th1"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "th.zip")
			_ = os.WriteFile(p, []byte("zipbytes"), 0o644)
			return p, nil
		},
		ThemePollInterval: time.Millisecond,
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	if !deployCalled {
		t.Fatal("Dashboard ExtensionDeploy was not called")
	}
	// Positive assertions that the UPDATE path actually fired (not create):
	// the generated version "2.0.0" (not "1.0.0") proves extID was non-empty and
	// newVers was consulted — the exact condition that makes upsertTheme's update=true.
	if len(res.Extensions) != 1 {
		t.Fatalf("extensions = %+v", res.Extensions)
	}
	e := res.Extensions[0]
	if e.Type != "theme" || e.ExtensionID != "th1" || e.VersionID != "tv2" {
		t.Fatalf("theme ext = %+v, want {theme, th1, tv2}", e)
	}
	if e.Version != "2.0.0" {
		t.Fatalf("theme ext version = %q, want 2.0.0 (create path yields 1.0.0 — wrong path taken)", e.Version)
	}
	if res.Version != "v-gen-2" {
		t.Fatalf("DeployResult.Version = %q, want v-gen-2", res.Version)
	}
	// Key regression guard: the connection endpoint must NOT be called on update.
	mu.Lock()
	defer mu.Unlock()
	if connectHit {
		t.Fatal("deploy theme UPDATE path must NOT hit the connection endpoint")
	}
}

func TestDeploy_FunctionLeg(t *testing.T) {
	// ProjectRoot with extensions/<dir>/src/index.js (known content).
	root := t.TempDir()
	jsContent := "export default function(input){ return input; }\n"
	srcDir := filepath.Join(root, "extensions", "fn", "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(srcDir, "index.js"), []byte(jsContent), 0o644); err != nil {
		t.Fatal(err)
	}

	var ossSignCalled, deployCalled bool
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	// GenerateVersion -> app_version "v-gen-1" (function is an add -> create at 1.0.0).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	// The function leg OSS-uploads the wasm too (v1 parity: buildFunction's wasm
	// is OSS-uploaded and its URL sent as resource_url in the deploy body).
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		ossSignCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		deployCalled = true
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	// Partner server: functions/create -> SUCCESS; capture source_code sent.
	var gotSourceCode string
	partnerSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasSuffix(r.URL.Path, "/openapi/2025-06/functions/create") {
			t.Errorf("path = %q, want .../functions/create", r.URL.Path)
		}
		fields, _ := parseFunctionForm(t, r)
		gotSourceCode = fields["source_code"]
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"code":"SUCCESS","data":{"function_id":"fn1","version":"1.0.0","version_id":"fv1"}}`))
	}))
	defer partnerSrv.Close()

	// Inject BuildArtifact returning a temp .wasm (no real javy).
	wasmDir := t.TempDir()
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")
	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"),
		Store:     store, Partner: newFunctionPartner(t, partnerSrv.URL),
		HTTPClient: srv.Client(),
		PartnerID:  "p1", ClientID: "cid_1",
		ProjectRoot: root,
		Locals:      []LocalExt{{Dir: "fn", Name: "fn", Type: "function", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(wasmDir, "fn.wasm")
			_ = os.WriteFile(p, []byte("\x00asm\x01\x00\x00\x00WASM"), 0o644)
			return p, nil
		},
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	if !ossSignCalled {
		t.Fatal("function leg must OSS-upload the wasm (v1 parity: resource_url in the deploy body)")
	}
	if !deployCalled {
		t.Fatal("Dashboard ExtensionDeploy was not called")
	}
	if len(res.Extensions) != 1 {
		t.Fatalf("extensions = %+v", res.Extensions)
	}
	e := res.Extensions[0]
	if e.Type != "function" || e.ExtensionID != "fn1" || e.VersionID != "fv1" {
		t.Fatalf("function ext = %+v, want {function, fn1, fv1}", e)
	}
	if e.ResourceURL != "https://read.example/chick-extension/fn.wasm" {
		t.Fatalf("function ext must carry the OSS resource_url (v1 parity); got %q", e.ResourceURL)
	}
	if gotSourceCode != jsContent {
		t.Fatalf("source_code = %q, want index.js content %q", gotSourceCode, jsContent)
	}
}

// TestDeploy_CheckoutStaleIdNoRemote_Creates verifies the checkout commit-gate:
// when a local extension has a stale toml ExtensionID but GenerateVersion returns
// no matching version for it (newVers[extID]=="") AND there is no remote match,
// the CREATE endpoint is hit (not commit) with version "1.0.0".
func TestDeploy_CheckoutStaleIdNoRemote_Creates(t *testing.T) {
	mux := http.NewServeMux()
	// Remote: no extensions at all (the stale id finds no match in Diff).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	// GenerateVersion: no per-extension entries — newVers["ext_stale"] will be "".
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{
			"app_version": "v-gen-stale",
			"extensions":  []any{},
		}})
	})
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// create IS the expected path; capture its body and mark as hit.
	var mu sync.Mutex
	var createBody []byte
	var createHit bool
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		createBody = b
		createHit = true
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e_new","id":"v_new"}}}`))
	})
	// commit must NOT be hit; registering it lets us detect bugs vs. a raw 404.
	var commitHit bool
	mux.HandleFunc("/openapi/checkout_extensions/commit", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		commitHit = true
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"ext_stale","id":"v2"}}}`))
	})
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	dir := t.TempDir()
	d := NewDashboard(client.New(srv.URL), "ptok")
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")

	deps := DeployDeps{
		Dashboard: d, Store: store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		// Stale local id that does NOT appear in GenerateVersion's extensions.
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0", ExtensionID: "ext_stale"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
	res, ex := Deploy(context.Background(), deps)
	if ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}

	mu.Lock()
	defer mu.Unlock()
	if commitHit {
		t.Fatal("stale-id checkout must NOT hit the commit endpoint (no remote match + no generated version)")
	}
	if !createHit {
		t.Fatal("stale-id checkout must hit the create endpoint")
	}
	// Create payload must use version "1.0.0".
	var createEnv map[string]any
	if err := json.Unmarshal(createBody, &createEnv); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	createExt, _ := createEnv["extension"].(map[string]any)
	if createExt == nil || createExt["version"] != "1.0.0" {
		t.Fatalf("create body extension.version = %v, want 1.0.0; body = %s", createExt["version"], createBody)
	}
	if res.Version != "v-gen-stale" {
		t.Fatalf("DeployResult.Version = %q, want v-gen-stale", res.Version)
	}
}

func TestDeploy_UpsertFailure_NamesExtension(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	var ossURL string
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// GenerateVersion must succeed so the failure under test is the checkout upsert.
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	// checkout create fails -> the deploy error must name the failing extension.
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
		_, _ = w.Write([]byte(`{"message":"boom"}`))
	})
	srv := httptest.NewServer(mux)
	defer srv.Close()
	ossURL = srv.URL + "/upload"

	dir := t.TempDir()
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")
	deps := DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"),
		Store:     store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1",
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(dir, "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
	_, ex := Deploy(context.Background(), deps)
	if ex == nil {
		t.Fatal("expected error on upsert failure")
	}
	if ex.Detail == nil || !strings.Contains(ex.Detail.Message, "co") {
		t.Fatalf("deploy error must name the failing extension; message = %q", ex.Error())
	}
}
