package app

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestDevReport(t *testing.T) {
	var ossURL string
	var (
		mu          sync.Mutex
		devBody     map[string]any
		devCalled   bool
		genCalledIs string
	)

	mux := http.NewServeMux()
	// is_dev extension_versions -> no existing remotes (local checkout is new/create).
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	// OSS sign + upload.
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	// checkout create.
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e1","id":"v1id"}}}`))
	})
	// GenerateVersion (is_dev) -> app_version "v1".
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		genCalledIs = r.URL.Query().Get("is_dev")
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v1"}})
	})
	// ExtensionDev (/dev) -> install_url; capture the full envelope so we can
	// assert both top-level app fields and the extensions array.
	var devEnv map[string]any
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/dev", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		var env map[string]any
		_ = json.Unmarshal(b, &env)
		mu.Lock()
		devCalled = true
		devEnv = env
		if app, ok := env["app"].(map[string]any); ok {
			devBody = app
		}
		mu.Unlock()
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"install_url": "https://x/install"}})
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

	res, ex := DevReport(context.Background(), deps, "https://pub.example", "/auth", "/auth/callback")
	if ex != nil {
		t.Fatalf("DevReport: %v", ex)
	}
	if res.InstallURL != "https://x/install" {
		t.Fatalf("InstallURL = %q, want https://x/install", res.InstallURL)
	}
	if res.AppURL != "https://pub.example/auth" {
		t.Fatalf("AppURL = %q, want https://pub.example/auth", res.AppURL)
	}
	if res.RedirectURL != "https://pub.example/auth/callback" {
		t.Fatalf("RedirectURL = %q, want https://pub.example/auth/callback", res.RedirectURL)
	}
	if res.Version != "v1" {
		t.Fatalf("Version = %q, want v1", res.Version)
	}
	if len(res.Extensions) != 1 || res.Extensions[0].ExtensionID != "e1" {
		t.Fatalf("Extensions = %+v", res.Extensions)
	}

	mu.Lock()
	defer mu.Unlock()
	if !devCalled {
		t.Fatal("ExtensionDev (/dev) was not called")
	}
	if genCalledIs != "1" {
		t.Fatalf("GenerateVersion is_dev = %q, want 1", genCalledIs)
	}
	if devBody == nil {
		t.Fatal("ExtensionDev received no app payload")
	}
	if devBody["dev_app_uri"] != "https://pub.example/auth" {
		t.Fatalf("dev_app_uri = %v, want https://pub.example/auth", devBody["dev_app_uri"])
	}
	if devBody["dev_redirect_uri"] != "https://pub.example/auth/callback" {
		t.Fatalf("dev_redirect_uri = %v, want https://pub.example/auth/callback", devBody["dev_redirect_uri"])
	}
	if devBody["version"] != "v1" {
		t.Fatalf("version = %v, want v1", devBody["version"])
	}
	// Verify the dev payload carries extension_version + extension_version_id (v1 parity).
	_ = devEnv // captured above
	app, _ := devEnv["app"].(map[string]any)
	if app == nil {
		t.Fatal("dev payload: no 'app' key")
	}
	exts, _ := app["extensions"].([]any)
	if len(exts) == 0 {
		t.Fatal("dev payload: no extensions in body")
	}
	ext0, _ := exts[0].(map[string]any)
	if ext0 == nil {
		t.Fatal("dev payload: extensions[0] is not a map")
	}
	if ext0["extension_version_id"] == "" || ext0["extension_version_id"] == nil {
		t.Fatalf("dev payload extension_version_id is empty; ext = %v", ext0)
	}
	if ext0["extension_version"] == "" || ext0["extension_version"] == nil {
		t.Fatalf("dev payload extension_version is empty; ext = %v", ext0)
	}
}
