package app

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
)

func TestDashboard_GetPartners(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli/v2/partners" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Real shape: the partner display name is "business_name", not "name".
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"partners": []map[string]any{{"id": "p1", "business_name": "Acme"}}},
		})
	}))
	defer srv.Close()

	d := NewDashboard(client.New(srv.URL), "partner_tok")
	out, err := d.GetPartners(context.Background())
	if err != nil {
		t.Fatalf("GetPartners: %v", err)
	}
	if len(out.Partners) != 1 || out.Partners[0].ID != "p1" || out.Partners[0].BusinessName != "Acme" {
		t.Fatalf("partners = %+v (BusinessName should decode from business_name)", out)
	}
}

func TestDashboard_GetAppConfig_ReturnsSecretAndPartnerID(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/cli/v2/partners/p1/apps/cid_1" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		w.Header().Set("Content-Type", "application/json")
		// Real shape: app nested under "app"; secret field is "secret"; no
		// partner_id in the body (the caller's is carried through).
		_ = json.NewEncoder(w).Encode(map[string]any{
			"code": "Success",
			"data": map[string]any{"app": map[string]any{"client_id": "cid_1", "id": 9799759, "secret": "sec", "scopes": []string{"read"}}},
		})
	}))
	defer srv.Close()

	d := NewDashboard(client.New(srv.URL), "partner_tok")
	cfg, err := d.GetAppConfig(context.Background(), "p1", "cid_1")
	if err != nil {
		t.Fatalf("GetAppConfig: %v", err)
	}
	if cfg.ClientID != "cid_1" || cfg.ClientSecret != "sec" || cfg.PartnerID != "p1" {
		t.Fatalf("config = %+v", cfg)
	}
}

func TestDeployBody_StoreID(t *testing.T) {
	app := map[string]any{"version": "1.0.0"}

	// numeric store id -> present as a uint64 (the backend's store_id is uint64;
	// a string yields a 400, an absent value a 404 — v1 sends it as a number).
	b := deployBody(app, "365580")
	if got, ok := b["store_id"].(uint64); !ok || got != 365580 {
		t.Fatalf("store_id = %#v, want uint64(365580)", b["store_id"])
	}
	if _, ok := b["app"]; !ok {
		t.Fatal("deployBody must wrap the payload under \"app\"")
	}

	// empty / non-numeric store id -> store_id omitted entirely.
	for _, sid := range []string{"", "abc", "x365580"} {
		if _, present := deployBody(app, sid)["store_id"]; present {
			t.Fatalf("store_id must be omitted for %q", sid)
		}
	}
}

// TestExtensionDeploy_StoreIDIsJSONNumber wires a numeric store id through
// ExtensionDeploy and asserts the on-the-wire body carries store_id as a JSON
// number (not a string) — the backend's AppDeployForm.store_id is uint64 and
// rejects a string with a 400 (verified live; v1 parity).
func TestExtensionDeploy_StoreIDIsJSONNumber(t *testing.T) {
	var gotBody map[string]any
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		_ = json.NewDecoder(r.Body).Decode(&gotBody)
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{}})
	}))
	defer srv.Close()

	d := NewDashboard(client.New(srv.URL), "ptok")
	if _, err := d.ExtensionDeploy(context.Background(), "p1", "cid_1", "365580",
		map[string]any{"version": "1.0.0", "extensions": []any{}}); err != nil {
		t.Fatalf("ExtensionDeploy: %v", err)
	}
	// encoding/json decodes a JSON number into float64; a string would decode to
	// string and fail this assertion (catching the regression).
	if got, ok := gotBody["store_id"].(float64); !ok || got != 365580 {
		t.Fatalf("store_id on the wire = %#v, want JSON number 365580", gotBody["store_id"])
	}
}
