package checkout_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestDeploy_PostsWrappedVersionLevelBody(t *testing.T) {
	var gotPath string
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9") // --version 1.0 → server id V9
			return
		}
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"ok": true}})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "deploy", "--extension-id", "E1", "--version", "1.0"); err != nil {
		t.Fatalf("deploy: %v", err)
	}
	if gotPath != "/openapi/checkout_extensions/deploy" {
		t.Errorf("path = %q", gotPath)
	}
	ext := gotBody["extension"].(map[string]any) // wrapped
	if ext["extension_id"] != "E1" || ext["id"] != "V9" {
		t.Fatalf("deploy body = %v", gotBody)
	}
}

func TestDeploy_RequiresBothFlags(t *testing.T) {
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "deploy", "--extension-id", "E1") // missing version
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("missing --version → type=validation, got %v", err)
	}
}

// TestDeploy_BusinessFailureEnvelopeErrors locks the fix for the 200-OK
// failure envelope on the fire-and-print commands: checkout endpoints reject
// with HTTP 200 + {message, status != 0}, and deploy must surface that as an
// api-class error instead of printing {"ok":true,...} and exiting 0.
func TestDeploy_BusinessFailureEnvelopeErrors(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "INVALID_VERSION", "status": 3})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	err := execCheckout(t, f, out, "deploy", "--extension-id", "E1", "--version", "1.0")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError on a 200 business-failure envelope, got %v (stdout: %s)", err, out.String())
	}
	if ee.Code != output.ExitAPI || ee.Detail.Type != output.TypeAPI {
		t.Fatalf("want exit %d type %s, got exit %d type %s", output.ExitAPI, output.TypeAPI, ee.Code, ee.Detail.Type)
	}
	if !strings.Contains(ee.Detail.Message, "INVALID_VERSION") {
		t.Errorf("error must carry the server message, got %q", ee.Detail.Message)
	}
}

// TestDeploy_HTTPErrorCarriesEndpoint: a non-2xx response must name the
// failing method+path in error.detail (doAPI attaches WithEndpoint).
func TestDeploy_HTTPErrorCarriesEndpoint(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		w.WriteHeader(http.StatusBadRequest)
		_, _ = w.Write([]byte(`{"message":"bad request"}`))
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	err := execCheckout(t, f, out, "deploy", "--extension-id", "E1", "--version", "1.0")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Detail == nil {
		t.Fatalf("expected ExitError with endpoint detail, got %v", err)
	}
	if ee.Detail.Detail.Method != "POST" || ee.Detail.Detail.Path != "/openapi/checkout_extensions/deploy" {
		t.Fatalf("endpoint = %s %s", ee.Detail.Detail.Method, ee.Detail.Detail.Path)
	}
}

func TestUndeploy_ExtensionLevelNoVersion(t *testing.T) {
	var gotBody map[string]any
	var gotPath string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"ok": true}})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "undeploy", "--extension-id", "E1"); err != nil {
		t.Fatalf("undeploy: %v", err)
	}
	if gotPath != "/openapi/checkout_extensions/undeploy" {
		t.Errorf("path = %q", gotPath)
	}
	ext := gotBody["extension"].(map[string]any)
	if ext["extension_id"] != "E1" {
		t.Fatalf("undeploy body = %v", gotBody)
	}
	if _, hasID := ext["id"]; hasID {
		t.Error("undeploy is extension-level — must NOT send a version id")
	}
}
