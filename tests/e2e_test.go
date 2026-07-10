// Package tests contains end-to-end tests that start a mock HTTP server and
// invoke the CLI binary via exec.Command.  No real API or keychain is used.
package tests_test

import (
	"context"
	"encoding/json"
	"io/fs"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"shoplazza-cli-v2/internal/testenv"
)

// apiEnv returns the environment variables needed to run module commands in
// tests: an API base URL override and a pre-injected access token so the
// auth guard passes without a real keychain.
func apiEnv(apiBaseURL string) []string {
	return []string{
		"SHOPLAZZA_CLI_API_BASE_URL=" + apiBaseURL,
		"SHOPLAZZA_ACCESS_TOKEN=test_token",
	}
}

// buildBinary compiles the CLI binary into a temp dir and returns the path.
func buildBinary(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	bin := filepath.Join(dir, "shoplazza")
	// go test sets cwd to the package dir (tests/); navigate up to project root.
	projectRoot, err := filepath.Abs("..")
	if err != nil {
		t.Fatal(err)
	}
	cmd := exec.Command("go", "build", "-o", bin, ".")
	cmd.Dir = projectRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("build failed: %v\n%s", err, out)
	}
	return bin
}

// runCLI runs the CLI binary with the given args and environment overrides.
// Returns stdout, stderr, exit code.
func runCLI(t *testing.T, bin string, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

func TestProductsList_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/products" {
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"products": []any{
				map[string]any{
					"id":                 "gid_001",
					"title":              "Test Product",
					"published":          true,
					"vendor":             "TestVendor",
					"inventory_quantity": 10,
				},
			},
			"cursor":     "cursor_next",
			"pre_cursor": "",
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, stderr, code := runCLI(t, bin, env, "products", "list")
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}

	out := unwrapAPISuccess(t, stdout)
	products, ok := out["products"].([]any)
	if !ok || len(products) == 0 {
		t.Fatalf("expected products array, got: %v", out)
	}
	first := products[0].(map[string]any)
	if first["id"] != "gid_001" {
		t.Errorf("product[0].id = %v, want 'gid_001'", first["id"])
	}
}

func TestProductsList_WithFilters(t *testing.T) {
	var capturedQuery string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		capturedQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"products": []any{},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, _, code := runCLI(t, bin, env,
		"products", "list",
		"--params", `{"title":"shoe","published_status":"published","per_page":10}`,
	)
	if code != 0 {
		t.Fatalf("unexpected exit code %d", code)
	}

	// Verify query parameters are sent correctly.
	if !strings.Contains(capturedQuery, "title=shoe") {
		t.Errorf("query %q missing title=shoe", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "published_status=published") {
		t.Errorf("query %q missing published_status=published", capturedQuery)
	}
	if !strings.Contains(capturedQuery, "per_page=10") {
		t.Errorf("query %q missing per_page=10", capturedQuery)
	}
}

func TestProductsCount_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/products/count" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{"count": 42})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "products", "count")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stdout)
	}
	out := unwrapAPISuccess(t, stdout)
	if count, _ := out["count"].(float64); count != 42 {
		t.Errorf("count = %v, want 42", out["count"])
	}
}

func TestProductsGet_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/openapi/2026-01/products/gid_123" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		json.NewEncoder(w).Encode(map[string]any{
			"product": map[string]any{
				"id":    "gid_123",
				"title": "Single Product",
			},
		})
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	stdout, _, code := runCLI(t, bin, env, "products", "get", "--params", `{"product_id":"gid_123"}`)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stdout)
	}
	out := unwrapAPISuccess(t, stdout)
	product, _ := out["product"].(map[string]any)
	if product["id"] != "gid_123" {
		t.Errorf("product.id = %v", product["id"])
	}
}

func TestProductsDelete_NormalizesEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodDelete {
			t.Errorf("method = %s, want DELETE", r.Method)
		}
		if r.URL.Path != "/openapi/2026-01/products/gid_999" {
			t.Errorf("unexpected path: %s", r.URL.Path)
		}
		// API returns empty body on delete.
		w.WriteHeader(http.StatusOK)
	}))
	defer srv.Close()

	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	// The dynamic runner is a thin pass-through; an empty server body yields {}.
	stdout, _, code := runCLI(t, bin, env, "products", "delete", "--params", `{"product_id":"gid_999"}`)
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
}

func TestErrorOutput_IsJSONEnvelope(t *testing.T) {
	// Force a network error by pointing to a closed port.
	bin := buildBinary(t)
	env := apiEnv("http://127.0.0.1:1")

	_, stderr, code := runCLI(t, bin, env, "products", "list")
	if code == 0 {
		t.Fatal("expected non-zero exit code")
	}

	// stderr must be parseable JSON with ok=false.
	var env2 map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &env2); err != nil {
		t.Fatalf("stderr is not JSON: %v\nstderr: %s", err, stderr)
	}
	if ok, _ := env2["ok"].(bool); ok {
		t.Error("ok should be false")
	}
	if env2["error"] == nil {
		t.Error("error field should be present")
	}
}

func TestAuthStatus_NotLoggedIn(t *testing.T) {
	bin := buildBinary(t)
	// Isolate the config dir so we don't read real auth state. runCLI passes
	// os.Environ() to the subprocess, so t.Setenv-based isolation covers the
	// Windows %AppData% path too (HOME/XDG alone would not).
	testenv.IsolateConfigDir(t)
	env := []string{
		"SHOPLAZZA_ACCESS_TOKEN=",
		"SHOPLAZZA_UAT=",
	}

	stdout, _, code := runCLI(t, bin, env, "auth", "status")
	if code != 0 {
		t.Fatalf("exit %d: %s", code, stdout)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v", err)
	}
	if loggedIn, _ := out["logged_in"].(bool); loggedIn {
		t.Error("should not be logged in with empty config dir")
	}
}

func TestAuthLogin_UATPath_MockServer(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch r.URL.Path {
		case "/api/saiga/cli/auth/me":
			json.NewEncoder(w).Encode(map[string]any{"user_id": "u_e2e", "account": "e2e@example.com"})
		case "/api/saiga/cli/auth/exchange/store-at":
			json.NewEncoder(w).Encode(map[string]any{
				"access_token":   "at_e2e_token",
				"store_domain":   "e2e.myshoplazza.com",
				"store_id":       "store_e2e",
				"granted_scopes": []string{"read_products"},
				"at_expires_at":  "2026-05-01T00:00:00Z",
			})
		default:
			t.Errorf("unexpected path: %s", r.URL.Path)
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()

	bin := buildBinary(t)
	tmpHome := testenv.IsolateConfigDir(t)
	env := []string{
		"SHOPLAZZA_CLI_AUTH_BASE_URL=" + srv.URL,
	}

	stdout, stderr, code := runCLI(t, bin, env,
		"auth", "login", "--store-domain", "e2e.myshoplazza.com",
		"--uat", "uat_e2e_token",
	)
	if code != 0 {
		t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
	}
	var out map[string]any
	if err := json.Unmarshal([]byte(stdout), &out); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if out["ok"] != true {
		t.Errorf("ok = %v, want true", out["ok"])
	}
	if out["flow"] != "uat" {
		t.Errorf("flow = %v, want 'uat'", out["flow"])
	}

	authFile := findFile(t, tmpHome, "auth.json")
	if authFile == "" {
		t.Fatal("auth.json not found under tmpHome — login may not have persisted state")
	}
	data, err := os.ReadFile(authFile)
	if err != nil {
		t.Fatalf("read auth.json: %v", err)
	}
	if strings.Contains(string(data), "at_e2e_token") {
		t.Error("SECURITY: access token must not appear in plain auth.json")
	}
	if strings.Contains(string(data), "uat_e2e_token") {
		t.Error("SECURITY: UAT must not appear in plain auth.json")
	}
}

func TestAuthLogin_DomainNormalization(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"e2e.myshoplazza.com", "e2e.myshoplazza.com"},
		{"https://e2e.myshoplazza.com", "e2e.myshoplazza.com"},
		{"https://e2e.myshoplazza.com/", "e2e.myshoplazza.com"},
	}

	for _, tc := range cases {
		t.Run(tc.input, func(t *testing.T) {
			srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.Header().Set("Content-Type", "application/json")
				switch r.URL.Path {
				case "/api/saiga/cli/auth/me":
					json.NewEncoder(w).Encode(map[string]any{"account": "e2e@example.com"})
				case "/api/saiga/cli/auth/exchange/store-at":
					json.NewEncoder(w).Encode(map[string]any{
						"access_token":   "at_norm_token",
						"store_domain":   "e2e.myshoplazza.com",
						"store_id":       "store_norm",
						"granted_scopes": []string{"read_products"},
						"at_expires_at":  "2026-05-01T00:00:00Z",
					})
				default:
					t.Errorf("unexpected path: %s", r.URL.Path)
					http.NotFound(w, r)
				}
			}))
			defer srv.Close()

			bin := buildBinary(t)
			tmpHome := testenv.IsolateConfigDir(t)
			env := []string{
				"SHOPLAZZA_CLI_AUTH_BASE_URL=" + srv.URL,
			}

			stdout, stderr, code := runCLI(t, bin, env,
				"auth", "login", "--store-domain", tc.input,
				"--uat", "uat_norm_token",
			)
			if code != 0 {
				t.Fatalf("exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}

			configFile := findFile(t, tmpHome, "config.json")
			if configFile == "" {
				t.Fatal("config.json not written")
			}
			data, err := os.ReadFile(configFile)
			if err != nil {
				t.Fatalf("read config.json: %v", err)
			}
			content := string(data)
			if !strings.Contains(content, `"storeDomain": "`+tc.want+`"`) {
				t.Errorf("config.json storeDomain mismatch\nwant: %q\ngot: %s", tc.want, content)
			}
			if strings.Contains(content, "https://e2e") || strings.Contains(content, "http://e2e") {
				t.Error("store_domain must not contain URL scheme")
			}
		})
	}
}

// unwrapAPISuccess parses stdout as a {"ok":true,"data":...} envelope (the
// shape produced by output.PrintAPISuccess on spec-leaf and `api rest` calls)
// and returns the inner data map. Falls back to the raw parsed map if stdout
// isn't envelope-shaped — auth commands, for instance, write their own JSON.
func unwrapAPISuccess(t *testing.T, stdout string) map[string]any {
	t.Helper()
	var raw map[string]any
	if err := json.Unmarshal([]byte(stdout), &raw); err != nil {
		t.Fatalf("stdout not JSON: %v\n%s", err, stdout)
	}
	if ok, _ := raw["ok"].(bool); ok {
		if data, isMap := raw["data"].(map[string]any); isMap {
			return data
		}
	}
	return raw
}

// findFile walks root and returns the first file named `name`, or "" if not found.
// Used to locate auth.json regardless of platform-specific config dir layout
// (macOS uses ~/Library/Application Support; Linux uses ~/.config).
func findFile(t *testing.T, root, name string) string {
	t.Helper()
	var found string
	_ = filepath.WalkDir(root, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return nil
		}
		if d.Name() == name {
			found = path
			return fs.SkipAll
		}
		return nil
	})
	return found
}
