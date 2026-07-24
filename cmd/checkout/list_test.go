package checkout_test

import (
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestList_DefaultPublishedOnly(t *testing.T) {
	var gotStatus string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotStatus = r.URL.Query().Get("status")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"extensions": []map[string]any{
				{"name": "Alpha", "extension_id": "E1", "publish_status": "published"},
			}},
		})
	}))
	defer srv.Close()

	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "list"); err != nil {
		t.Fatalf("list: %v", err)
	}
	if gotStatus != "published" {
		t.Errorf("default list status = %q, want published", gotStatus)
	}
	var env map[string]any
	if err := json.Unmarshal(out.Bytes(), &env); err != nil {
		t.Fatal(err)
	}
	data := env["data"].(map[string]any)
	exts := data["extensions"].([]any)
	if exts[0].(map[string]any)["extension_id"] != "E1" {
		t.Fatalf("unexpected body: %s", out.String())
	}
}

func TestList_AllOmitsStatus(t *testing.T) {
	var hadStatus bool
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_, hadStatus = r.URL.Query()["status"]
		w.Header().Set("Content-Type", "application/json") // DoRaw respects Content-Type; required for envelope unwrap
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "list", "--all"); err != nil {
		t.Fatalf("list --all: %v", err)
	}
	if hadStatus {
		t.Error("--all must NOT send a status param")
	}
}

// ── versions ──────────────────────────────────────────────────────────────────

func TestVersions_RequiresExtensionID(t *testing.T) {
	f, out := tempCheckoutFactory(t, "http://unused")
	err := execCheckout(t, f, out, "versions")
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("missing --extension-id → type=validation, got %v", err)
	}
}

func TestVersions_Success(t *testing.T) {
	var gotExtID string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotExtID = r.URL.Query().Get("extension_id")
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"extensions": []map[string]any{
				{"version": "1.2", "id": "V9", "publish_status": "draft"},
			}},
		})
	}))
	defer srv.Close()
	f, out := tempCheckoutFactory(t, srv.URL)
	if err := execCheckout(t, f, out, "versions", "--extension-id", "E1"); err != nil {
		t.Fatalf("versions: %v", err)
	}
	if gotExtID != "E1" {
		t.Errorf("extension_id query = %q", gotExtID)
	}
}
