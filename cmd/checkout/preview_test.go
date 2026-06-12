package checkout_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestPreview_RequiresBothFlags(t *testing.T) {
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "preview", "--extension-id", "E1") // missing --version
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("missing --version → type=validation, got %v", err)
	}
}

// TestPreview_RealStatusEnvelope locks the REAL server shape: success is
// signalled via status/message (NOT code:"Success"/ok), so the client does NOT
// unwrap, and checkout_url sits under .data. Regression guard for the bug where
// preview read the wrong level and reported "missing checkout_url".
func TestPreview_RealStatusEnvelope(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data":    map[string]any{"checkout_url": "/checkout/realshape"},
			"errors":  []any{},
			"message": "success",
			"status":  0,
		})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "preview", "--extension-id", "E1", "--version", "1.0"); err != nil {
		t.Fatalf("preview (real envelope): %v", err)
	}
	var env map[string]any
	_ = json.Unmarshal(out.Bytes(), &env)
	url, _ := env["preview_url"].(string)
	if !strings.HasPrefix(url, "https://test-store.myshoplaza.com/checkout/realshape") {
		t.Errorf("preview_url not built from real-envelope checkout_url: %q", url)
	}
}

// TestPreview_BusinessFailureSurfacesServerMessage locks the fix for the
// 200-OK failure envelope on preview: {"message":...,"status":3} must surface
// as an api-class error carrying the server message — previously it fell
// through to internal "preview response missing checkout_url" (exit 5).
func TestPreview_BusinessFailureSurfacesServerMessage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"message": "PREVIEW_DENIED_BY_SERVER", "status": 3})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	err := execCheckout(t, f, out, "preview", "--extension-id", "E1", "--version", "1.0")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected ExitError, got %v", err)
	}
	if ee.Code != output.ExitAPI || ee.Detail.Type != output.TypeAPI {
		t.Fatalf("want exit %d type %s, got exit %d type %s", output.ExitAPI, output.TypeAPI, ee.Code, ee.Detail.Type)
	}
	if !strings.Contains(ee.Detail.Message, "PREVIEW_DENIED_BY_SERVER") {
		t.Errorf("server message lost: %q", ee.Detail.Message)
	}
}

// TestPreview_InvalidStoreDomainIsValidation: an unparsable store domain must
// yield a validation error, not a nil-deref panic, when the API base URL is
// decoupled from the store domain (as with SHOPLAZZA_CLI_API_BASE_URL).
func TestPreview_InvalidStoreDomainIsValidation(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"checkout_url": "/c/x"}, "message": "success", "status": 0,
		})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL) // client base = srv, store domain separate
	f.Config.StoreDomain = "bad domain.com"   // space → url.Parse fails
	err := execCheckout(t, f, out, "preview", "--extension-id", "E1", "--version", "1.0")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("invalid store domain must be type=validation, got %v", err)
	}
}

func TestPreview_BuildsURLFromCurrentStore(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/openapi/checkout_extensions/version/list" {
			writeCheckoutVersionList(w, "1.0", "V9")
			return
		}
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"checkout_url": "/checkout/abc123"}, // relative → resolved against store
		})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL) // Config.StoreDomain = test-store.myshoplaza.com
	if err := execCheckout(t, f, out, "preview", "--extension-id", "E1", "--version", "1.0"); err != nil {
		t.Fatalf("preview: %v", err)
	}
	ext := gotBody["extension"].(map[string]any)
	if ext["extension_id"] != "E1" || ext["id"] != "V9" {
		t.Fatalf("preview POST body must be wrapped {extension:{extension_id,id}}, got %v", gotBody)
	}
	var env map[string]any
	_ = json.Unmarshal(out.Bytes(), &env)
	url, _ := env["preview_url"].(string)
	if !strings.HasPrefix(url, "https://test-store.myshoplaza.com/checkout/abc123") {
		t.Errorf("preview_url base wrong: %q", url)
	}
	if !strings.Contains(url, "step=contact_information") {
		t.Errorf("preview_url missing step param: %q", url)
	}
}
