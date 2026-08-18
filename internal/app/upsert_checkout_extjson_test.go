package app

// Tests for loadCheckoutExtJSON / mergeCheckoutExtJSON / writeBackExtensionJSONID
// and their wiring into the deploy checkout leg.

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

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func writeExtJSON(t *testing.T, root, dir, content string) {
	t.Helper()
	d := filepath.Join(root, "extensions", dir)
	if err := os.MkdirAll(d, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(d, "extension.json"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestLoadCheckoutExtJSON_MissingFileReturnsNil(t *testing.T) {
	cfg, err := loadCheckoutExtJSON(t.TempDir(), "co")
	if err != nil || cfg != nil {
		t.Fatalf("missing file: cfg=%v err=%v, want nil/nil", cfg, err)
	}
}

func TestLoadCheckoutExtJSON_EmptyProjectRootIsNoop(t *testing.T) {
	cfg, err := loadCheckoutExtJSON("", "co")
	if err != nil || cfg != nil {
		t.Fatalf("empty projectRoot: cfg=%v err=%v, want nil/nil", cfg, err)
	}
}

func TestLoadCheckoutExtJSON_MalformedIsValidationError(t *testing.T) {
	for _, content := range []string{"{not json", "null", "[1,2]", `"str"`} {
		root := t.TempDir()
		writeExtJSON(t, root, "co", content)
		_, err := loadCheckoutExtJSON(root, "co")
		if err == nil || err.Detail.Type != output.TypeValidation {
			t.Errorf("content %q: want validation error, got %v", content, err)
		}
	}
}

func TestLoadCheckoutExtJSON_PreservesIntegerPrecision(t *testing.T) {
	root := t.TempDir()
	writeExtJSON(t, root, "co", `{"placeholder":{"ts":1755158400123456789}}`)
	cfg, err := loadCheckoutExtJSON(root, "co")
	if err != nil {
		t.Fatal(err)
	}
	inner := map[string]any{}
	if mErr := mergeCheckoutExtJSON(inner, cfg, "1.0.0", ""); mErr != nil {
		t.Fatal(mErr)
	}
	if ef := inner["extends_fields"].(string); !strings.Contains(ef, "1755158400123456789") {
		t.Errorf("large integer lost precision: %s", ef)
	}
}

func TestMergeCheckoutExtJSON_NilCfgIsNoop(t *testing.T) {
	inner := map[string]any{"version": "1.0.0"}
	if err := mergeCheckoutExtJSON(inner, nil, "1.0.0", ""); err != nil {
		t.Fatal(err)
	}
	if len(inner) != 1 {
		t.Fatalf("inner gained keys without cfg: %v", inner)
	}
}

func TestMergeCheckoutExtJSON_OverridesVersionAndID(t *testing.T) {
	cfg := map[string]any{
		"version": "9.9.9", "deleteTarget": []any{"navigate"}, "placeholder": map[string]any{},
		"templateName": "checkout", "themeName": "", "extensionId": "stale-id", "extensionDescription": "desc",
	}
	inner := map[string]any{"resource_url": "u", "version": "2.0.0", "name": "co"}
	if err := mergeCheckoutExtJSON(inner, cfg, "2.0.0", "real-id"); err != nil {
		t.Fatal(err)
	}
	var ef map[string]any
	if err := json.Unmarshal([]byte(inner["extends_fields"].(string)), &ef); err != nil {
		t.Fatalf("extends_fields is not JSON: %v", err)
	}
	if ef["version"] != "2.0.0" || ef["extensionId"] != "real-id" {
		t.Errorf("version/extensionId not overridden: %v", ef)
	}
	if dt, _ := ef["deleteTarget"].([]any); len(dt) != 1 || dt[0] != "navigate" {
		t.Errorf("deleteTarget lost: %v", ef["deleteTarget"])
	}
	if inner["template_name"] != "checkout" || inner["description"] != "desc" {
		t.Errorf("promoted fields wrong: %v", inner)
	}
	// cfg itself must stay pristine (it is reused for the file write-back).
	if cfg["version"] != "9.9.9" || cfg["extensionId"] != "stale-id" {
		t.Errorf("merge mutated caller's cfg: %v", cfg)
	}
}

// Create path: a stale file id must not ride into extends_fields, and empty
// promoted fields must be omitted (not sent as "").
func TestMergeCheckoutExtJSON_CreatePathBlanksIDAndOmitsEmpties(t *testing.T) {
	cfg := map[string]any{"deleteTarget": []any{"navigate"}, "extensionId": "foreign-id", "themeName": ""}
	inner := map[string]any{"resource_url": "u", "version": "1.0.0", "name": "co"}
	if err := mergeCheckoutExtJSON(inner, cfg, "1.0.0", ""); err != nil {
		t.Fatal(err)
	}
	var ef map[string]any
	_ = json.Unmarshal([]byte(inner["extends_fields"].(string)), &ef)
	if ef["extensionId"] != "" {
		t.Errorf("create path must blank extensionId, got %v", ef["extensionId"])
	}
	for _, k := range []string{"template_name", "theme_name", "description"} {
		if _, ok := inner[k]; ok {
			t.Errorf("empty %s must be omitted, got %v", k, inner[k])
		}
	}
}

func TestWriteBackExtensionJSONID(t *testing.T) {
	root := t.TempDir()
	writeExtJSON(t, root, "co", `{"version":"1.0","deleteTarget":["navigate"],"extensionId":""}`)
	cfg, _ := loadCheckoutExtJSON(root, "co")
	if err := writeBackExtensionJSONID(root, "co", "new-id", cfg); err != nil {
		t.Fatal(err)
	}
	got, _ := loadCheckoutExtJSON(root, "co")
	if got["extensionId"] != "new-id" || got["version"] != "1.0" {
		t.Fatalf("write-back result: %v", got)
	}
}

func newCheckoutDeployMux(t *testing.T, createBody *[]byte, mu *sync.Mutex) (*http.ServeMux, *string) {
	t.Helper()
	mux := http.NewServeMux()
	ossURL := new(string)
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/extension_versions", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	})
	mux.HandleFunc("/openapi/checkout_extensions/file/sign", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{
			"write_host": *ossURL, "read_host": "https://read.example/", "policy": "P", "access_id": "AK", "sign": "SG"})
	})
	mux.HandleFunc("/upload", func(w http.ResponseWriter, r *http.Request) { w.WriteHeader(http.StatusOK) })
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/version/generate", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"app_version": "v-gen-1", "extensions": []any{}}})
	})
	mux.HandleFunc("/openapi/checkout_extensions/create", func(w http.ResponseWriter, r *http.Request) {
		b, _ := io.ReadAll(r.Body)
		mu.Lock()
		*createBody = b
		mu.Unlock()
		_, _ = w.Write([]byte(`{"data":{"extension":{"extension_id":"e1","id":"v1"}}}`))
	})
	mux.HandleFunc("/api/cli/v2/partners/p1/apps/cid_1/deploy", func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"version": "1.0.0", "name": "MyApp"}})
	})
	return mux, ossURL
}

func checkoutDeployDeps(srv *httptest.Server, root string) DeployDeps {
	store := client.New(srv.URL)
	store.SetBearerToken("store-tok")
	return DeployDeps{
		Dashboard: NewDashboard(client.New(srv.URL), "ptok"), Store: store, HTTPClient: srv.Client(),
		PartnerID: "p1", ClientID: "cid_1", ProjectRoot: root,
		Locals: []LocalExt{{Dir: "co", Name: "co", Type: "checkout", Version: "1.0.0"}},
		BuildArtifact: func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
			p := filepath.Join(root, "co.js")
			_ = os.WriteFile(p, []byte("bundle"), 0o644)
			return p, nil
		},
	}
}

// End-to-end through Deploy: the create body carries extends_fields, and the
// server-issued id is written back into extension.json.
func TestDeploy_CheckoutCarriesExtendsFields(t *testing.T) {
	var mu sync.Mutex
	var createBody []byte
	mux, ossURL := newCheckoutDeployMux(t, &createBody, &mu)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	*ossURL = srv.URL + "/upload"

	root := t.TempDir()
	writeExtJSON(t, root, "co", `{"version":"1.0","deleteTarget":["navigate"],"templateName":"checkout"}`)
	if err := os.WriteFile(filepath.Join(root, "extensions", "co", "shoplazza.extension.toml"),
		[]byte("name = \"co\"\ntype = \"checkout\"\n"), 0o644); err != nil {
		t.Fatal(err)
	}

	if _, ex := Deploy(context.Background(), checkoutDeployDeps(srv, root)); ex != nil {
		t.Fatalf("Deploy: %v", ex)
	}
	mu.Lock()
	b := createBody
	mu.Unlock()
	var env map[string]any
	if err := json.Unmarshal(b, &env); err != nil {
		t.Fatalf("unmarshal create body: %v", err)
	}
	ext, _ := env["extension"].(map[string]any)
	ef, _ := ext["extends_fields"].(string)
	if !strings.Contains(ef, `"deleteTarget":["navigate"]`) || !strings.Contains(ef, `"version":"1.0.0"`) {
		t.Errorf("extends_fields wrong: %s", ef)
	}
	if ext["template_name"] != "checkout" {
		t.Errorf("template_name = %v, want checkout", ext["template_name"])
	}
	// Server-issued id written back into extension.json (checkout push parity).
	cfg, _ := loadCheckoutExtJSON(root, "co")
	if cfg["extensionId"] != "e1" {
		t.Errorf("extension.json id not written back, got %v", cfg["extensionId"])
	}
}

// A malformed extension.json must fail BEFORE the build/upload legs run.
func TestDeploy_CheckoutMalformedExtJSONFailsBeforeBuild(t *testing.T) {
	var mu sync.Mutex
	var createBody []byte
	mux, ossURL := newCheckoutDeployMux(t, &createBody, &mu)
	srv := httptest.NewServer(mux)
	defer srv.Close()
	*ossURL = srv.URL + "/upload"

	root := t.TempDir()
	writeExtJSON(t, root, "co", "null")

	built := false
	deps := checkoutDeployDeps(srv, root)
	deps.BuildArtifact = func(ctx context.Context, l LocalExt) (string, *output.ExitError) {
		built = true
		return "", output.ErrInternal("must not be called")
	}
	_, ex := Deploy(context.Background(), deps)
	if ex == nil || ex.Detail.Type != output.TypeValidation {
		t.Fatalf("want validation error, got %v", ex)
	}
	if built {
		t.Error("BuildArtifact ran before extension.json validation")
	}
}
