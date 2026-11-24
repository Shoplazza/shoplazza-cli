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

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
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

// buildBinary returns the package-shared CLI binary (compiled once by
// sharedBinary in contract_smoke_test.go).
func buildBinary(t *testing.T) string {
	t.Helper()
	return sharedBinary(t)
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

// Shortcut time flags go through one parser: --since/--until reach the
// server as RFC3339 UTC, --start/--end as unix seconds, and a bad value exits
// 2 with the flag named, before any request is sent.
func TestSpecLeafInput_MockServer(t *testing.T) {
	var (
		hits  int
		path  string
		query map[string][]string
		body  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		path = r.URL.EscapedPath()
		query = r.URL.Query()
		body = nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	_, stderr, code := runCLI(t, bin, env, "themes", "block", "add",
		"--params", `{"oseid":"os1","section_id":"s1"}`, "--data", `{"doc_id":"d1","theme_id":"T1","block":{"type":"text"}}`)
	if code != 0 || hits != 1 {
		t.Fatalf("theme_id in --data: exit %d, hits %d, stderr=%s", code, hits, stderr)
	}
	if body["theme_id"] != "T1" || len(query) != 0 {
		t.Errorf("theme_id in --data: body.theme_id=%v query=%v, want T1 and no query", body["theme_id"], query)
	}

	for _, c := range []struct {
		args     []string
		wantPath string
	}{
		{[]string{"orders", "get-by-number", "--params", `{"number":"#1001"}`}, "/openapi/2026-01/orders/number/1001"},
		{[]string{"discounts", "get-by-code", "--params", `{"discount_code":"#A?B%"}`}, "/openapi/2026-01/discounts/by-code/%23A%3FB%25"},
	} {
		if _, stderr, code := runCLI(t, bin, env, c.args...); code != 0 || path != c.wantPath {
			t.Errorf("%v: exit %d, path %q, want %q; stderr=%s", c.args, code, path, c.wantPath, stderr)
		}
	}
	stdout, _, code := runCLI(t, bin, env, "orders", "get-by-number", "--params", `{"number":"#1001"}`, "--dry-run")
	if code != 0 || !strings.Contains(stdout, `"path": "/openapi/2026-01/orders/number/1001"`) ||
		!strings.Contains(stdout, `dropped the leading # from number`) {
		t.Errorf("get-by-number dry-run: exit %d, stdout=%s", code, stdout)
	}

	bad := []struct {
		args           []string
		subtype, param string
	}{
		{[]string{"themes", "block", "add", "--params", `{"oseid":"os1","section_id":"s1","theme_id":"T1"}`,
			"--data", `{"doc_id":"d1","block":{"type":"text"}}`}, "misplaced_param", "theme_id"},
		{[]string{"themes", "app", "enable", "--params", `{"oseid":"os1","theme_id":"T1"}`,
			"--data", `{"block_id":"b1"}`, "--dry-run"}, "misplaced_param", "theme_id"},
		{[]string{"themes", "block", "add", "--params", `{"oseid":"os1","section_id":"s1"}`, "--data", `{}`, "--dry-run"},
			"missing_required_field", "doc_id"},
		{[]string{"themes", "section", "set-props", "--params", `{"oseid":"os1","section_id":"s1"}`,
			"--data", `{"doc_id":"d1","props":{}}`}, "missing_required_field", "theme_id"},
		{[]string{"discounts", "list", "--params", `{"per_page":5}`}, "unknown_param", "per_page"},
		{[]string{"orders", "get", "--params", `{"id":"o1"}`, "--dry-run"}, "unknown_param", "id"},
	}
	for _, c := range bad {
		before := hits
		stdout, stderr, code := runCLI(t, bin, env, c.args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2; stdout=%s stderr=%s", c.args, code, stdout, stderr)
			continue
		}
		if hits != before {
			t.Errorf("%v: a request was sent", c.args)
		}
		var e struct {
			Error struct{ Type, Subtype, Param string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(stderr), &e); err != nil {
			t.Errorf("%v: stderr is not a JSON envelope: %s", c.args, stderr)
			continue
		}
		if e.Error.Type != "validation" || e.Error.Subtype != c.subtype || e.Error.Param != c.param {
			t.Errorf("%v: error = %+v, want validation/%s on %s", c.args, e.Error, c.subtype, c.param)
		}
	}
}

func TestShortcutListAndEnumFlags_MockServer(t *testing.T) {
	var (
		hits  int
		query map[string][]string
		body  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		query = r.URL.Query()
		body = nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	for _, c := range []struct {
		args  []string
		query map[string][]string
	}{
		{[]string{"orders", "+search", "--financial-status", "paid,refunded"}, map[string][]string{"financial_status": {"paid", "refunded"}}},
		{[]string{"orders", "+count", "--status", "placed", "--status", "finished", "--fulfillment-status", "returning"},
			map[string][]string{"status": {"placed", "finished"}, "fulfillment_status": {"returning"}}},
		{[]string{"discounts", "+search", "--progress", "ongoing, paused"}, map[string][]string{"progress": {"ongoing", "paused"}}},
	} {
		if _, stderr, code := runCLI(t, bin, env, c.args...); code != 0 {
			t.Errorf("%v: exit %d, stderr=%s", c.args, code, stderr)
			continue
		}
		for k, want := range c.query {
			if got := query[k]; strings.Join(got, "|") != strings.Join(want, "|") {
				t.Errorf("%v: %s = %q, want %q", c.args, k, got, want)
			}
		}
	}
	_, stderr, code := runCLI(t, bin, env, "discounts", "+percent-code", "--target", "product", "--percent-off", "10",
		"--code", "T", "--variants", "a", "--variants", "b,c")
	discount, _ := body["discount"].(map[string]any)
	scope, _ := discount["entitled_product"].(map[string]any)
	if ids, _ := json.Marshal(scope["variant_ids"]); code != 0 || string(ids) != `["a","b","c"]` {
		t.Errorf("--variants a --variants b,c: exit %d, variant_ids=%s, stderr=%s", code, ids, stderr)
	}

	bad := []struct {
		args  []string
		param string
	}{
		{[]string{"orders", "+search", "--status", "bogus"}, "--status"},
		{[]string{"orders", "+count", "--financial-status", "paid,bogus"}, "--financial-status"},
		{[]string{"discounts", "+search", "--discount-type", "nope"}, "--discount-type"},
		{[]string{"discounts", "+percent-code", "--target", "order", "--target", "product", "--percent-off", "10"}, "--target"},
		{[]string{"discounts", "+percent-code", "--target", "order", "--percent-off", "10", "--percent-off", "20"}, "--percent-off"},
	}
	for _, c := range bad {
		before := hits
		_, stderr, code := runCLI(t, bin, env, c.args...)
		if code != 2 || hits != before {
			t.Errorf("%v: exit %d (want 2), request sent=%v; stderr=%s", c.args, code, hits != before, stderr)
			continue
		}
		var e struct {
			Error struct{ Type, Subtype, Param string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(stderr), &e); err != nil || e.Error.Subtype != "invalid_flag_value" || e.Error.Param != c.param {
			t.Errorf("%v: error = %+v (%v), want invalid_flag_value on %s", c.args, e.Error, err, c.param)
		}
	}
}

func TestShortcutTimeFlags_MockServer(t *testing.T) {
	var (
		hits  int
		query map[string][]string
		body  map[string]any
	)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		hits++
		query = r.URL.Query()
		body = nil
		_ = json.NewDecoder(r.Body).Decode(&body)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	bin := buildBinary(t)
	env := apiEnv(srv.URL)

	yesterday := time.Now().UTC().Truncate(24*time.Hour).AddDate(0, 0, -1).Format(time.RFC3339)
	ok := []struct {
		args  []string
		query map[string]string
		info  map[string]float64
	}{
		{[]string{"orders", "+search", "--since", "yesterday", "--until", "now"},
			map[string]string{"placed_at_min": yesterday}, nil},
		{[]string{"orders", "+count", "--since", "2026-11-01T00:00:00+08:00", "--until", "1800000000"},
			map[string]string{"placed_at_min": "2026-10-31T16:00:00Z", "placed_at_max": "2027-01-15T08:00:00Z"}, nil},
		{[]string{"customers", "+search", "--since", "2026-09-01", "--until", "2026-09-30 23:59"},
			map[string]string{"created_at_min": "2026-09-01T00:00:00Z", "created_at_max": "2026-09-30T23:59:00Z"}, nil},
		{[]string{"discounts", "+percent-code", "--target", "order", "--percent-off", "10", "--code", "T",
			"--start", "2027-06-30T16:00:00Z", "--end", "2027-07-01 00:00"},
			nil, map[string]float64{"starts_at": 1814371200, "ends_at": 1814400000}},
		{[]string{"discounts", "+rebate", "--target", "order", "--tiers", "100:10",
			"--start", "2027-06-30T16:00:00+08:00", "--end", "forever"},
			nil, map[string]float64{"starts_at": 1814342400, "ends_at": -1}},
	}
	for _, c := range ok {
		_, stderr, code := runCLI(t, bin, env, c.args...)
		if code != 0 {
			t.Errorf("%v: exit %d, stderr=%s", c.args, code, stderr)
			continue
		}
		for k, want := range c.query {
			if got := strings.Join(query[k], ","); got != want {
				t.Errorf("%v: %s = %q, want %q", c.args, k, got, want)
			}
		}
		if c.args[0] == "orders" && c.args[3] == "yesterday" {
			if _, err := time.Parse(time.RFC3339, strings.Join(query["placed_at_max"], "")); err != nil {
				t.Errorf("%v: placed_at_max %q is not RFC3339", c.args, query["placed_at_max"])
			}
		}
		discount, _ := body["discount"].(map[string]any)
		info, _ := discount["discount_info"].(map[string]any)
		for k, want := range c.info {
			if info[k] != want {
				t.Errorf("%v: discount_info.%s = %v, want %v", c.args, k, info[k], want)
			}
		}
	}

	bad := []struct {
		args  []string
		param string
	}{
		{[]string{"orders", "+search", "--since", "yesterdy"}, "--since"},
		{[]string{"orders", "+search", "--until", "1700000000000"}, "--until"},
		{[]string{"orders", "+count", "--since", "-1"}, "--since"},
		{[]string{"customers", "+search", "--since", "2026-11-02", "--until", "2026-11-01"}, "--since"},
		{[]string{"discounts", "+flashsale", "--value", "20", "--start", "2027-06-31"}, "--start"},
		{[]string{"discounts", "+mn-discount", "--tiers", "2:30", "--start", "2027-07-01", "--end", "2027-06-30"}, "--end"},
	}
	for _, c := range bad {
		before := hits
		_, stderr, code := runCLI(t, bin, env, c.args...)
		if code != 2 {
			t.Errorf("%v: exit %d, want 2; stderr=%s", c.args, code, stderr)
			continue
		}
		if hits != before {
			t.Errorf("%v: a request was sent despite the bad value", c.args)
		}
		var env struct {
			Error struct{ Type, Subtype, Param string } `json:"error"`
		}
		if err := json.Unmarshal([]byte(stderr), &env); err != nil {
			t.Errorf("%v: stderr is not a JSON envelope: %s", c.args, stderr)
			continue
		}
		if env.Error.Type != "validation" || env.Error.Subtype != "invalid_flag_value" || env.Error.Param != c.param {
			t.Errorf("%v: error = %+v, want validation/invalid_flag_value on %s", c.args, env.Error, c.param)
		}
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

func TestUnknownSubcommand_IsUsageError(t *testing.T) {
	bin := buildBinary(t)
	env := apiEnv("http://127.0.0.1:1")
	const mean = "\n\nDid you mean this?\n\t"
	const unknownCmd, unknownFlag = "unknown_command", "unknown_flag"

	tests := []struct {
		name    string
		args    []string
		subtype string
		message string
		hint    string
	}{
		{"root", []string{"prodd"}, unknownCmd, `unknown command "prodd" for "shoplazza"`, "'shoplazza --help'"},
		{"root near name", []string{"prodcts"}, unknownCmd, `unknown command "prodcts" for "shoplazza"` + mean + "products", "'shoplazza --help'"},
		{"module", []string{"products", "aa"}, unknownCmd, `unknown command "aa" for "shoplazza products"`, "'shoplazza products --help'"},
		{"near name", []string{"products", "variant"}, unknownCmd, `unknown command "variant" for "shoplazza products"` + mean + "variants", "'shoplazza products --help'"},
		{"nested group", []string{"products", "variants", "aa"}, unknownCmd, `unknown command "aa" for "shoplazza products variants"`, "'shoplazza products variants --help'"},
		{"after a flag", []string{"products", "--format", "json", "aa"}, unknownCmd, `unknown command "aa" for "shoplazza products"`, "'shoplazza products --help'"},
		{"with --help", []string{"products", "aa", "--help"}, unknownCmd, `unknown command "aa" for "shoplazza products"`, "'shoplazza products --help'"},
		{"static group", []string{"auth", "aa"}, unknownCmd, `unknown command "aa" for "shoplazza auth"`, "'shoplazza auth --help'"},
		{"other group, followed down", []string{"variants", "list"}, unknownCmd, `unknown command "variants" for "shoplazza"` + mean + "shoplazza products variants list", "'shoplazza --help'"},
		{"other group, leaf flag after it", []string{"variants", "list", "--params", "{}"}, unknownCmd, `unknown command "variants" for "shoplazza"` + mean + "shoplazza products variants list", "'shoplazza --help'"},
		{"other group from a group", []string{"products", "addresses"}, unknownCmd, `unknown command "addresses" for "shoplazza products"` + mean + "shoplazza customers addresses", "'shoplazza products --help'"},
		{"name shared by many", []string{"list"}, unknownCmd, `unknown command "list" for "shoplazza"`, "'shoplazza --help'"},
		{"flag near name", []string{"customers", "+search", "--emial", "a@b.c"}, unknownFlag, "unknown flag: --emial" + mean + "--email", "--help"},
		{"spec leaf flag near name", []string{"orders", "list", "--param", "{}"}, unknownFlag, "unknown flag: --param" + mean + "--params", "--help"},
		{"deprecated alias points at its canonical flag", []string{"discounts", "+percent-code", "--percnt", "10"}, unknownFlag, "unknown flag: --percnt" + mean + "--percent-off", "--help"},
		{"flag with no near name",[]string{"customers", "+search", "--xyz"}, unknownFlag, "unknown flag: --xyz", "--help"},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			stdout, stderr, code := runCLI(t, bin, env, tt.args...)
			if code != 2 || stdout != "" {
				t.Fatalf("exit %d, stdout %q; want exit 2 and empty stdout", code, stdout)
			}
			var got struct {
				Error struct{ Subtype, Message, Hint string }
			}
			if err := json.Unmarshal([]byte(stderr), &got); err != nil {
				t.Fatalf("stderr not JSON: %v\n%s", err, stderr)
			}
			e := got.Error
			if e.Subtype != tt.subtype || e.Message != tt.message || !strings.Contains(e.Hint, tt.hint) {
				t.Errorf("error = %+v; want %s, message %q, hint containing %q", e, tt.subtype, tt.message, tt.hint)
			}
		})
	}

	for _, args := range [][]string{{"products"}, {"products", "--help"}} {
		stdout, _, code := runCLI(t, bin, env, args...)
		if code != 0 || !strings.Contains(stdout, "shoplazza products [command]") {
			t.Errorf("%v: exit %d, stdout %q; want exit 0 and the group's help", args, code, stdout)
		}
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
