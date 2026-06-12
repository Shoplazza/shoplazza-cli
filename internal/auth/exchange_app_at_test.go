package auth

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"shoplazza-cli-v2/internal/client"
)

func TestExchangeAppAT_SendsFourFields(t *testing.T) {
	var got exchangeAppATRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/api/saiga/cli/auth/exchange/app-at" {
			t.Fatalf("path = %s", r.URL.Path)
		}
		_ = json.NewDecoder(r.Body).Decode(&got)
		w.Header().Set("Content-Type", "application/json")
		// partner_id is a uint64 carried as a string; at_expires_at is a
		// Timestamp serialized by protojson as RFC3339.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"access_token":  "app_at_xyz",
			"partner_id":    "42",
			"client_id":     "cid_1",
			"at_expires_at": "2030-01-01T00:00:00Z",
		})
	}))
	defer srv.Close()

	m := &Manager{Client: client.New(srv.URL)}
	// partner_id is sent as a string ("42") — protojson's canonical uint64 form.
	block, err := m.exchangeAppAT(context.Background(), "uat_1", "cid_1", "secret_1", "42")
	if err != nil {
		t.Fatalf("exchangeAppAT: %v", err)
	}
	if got.UAT != "uat_1" || got.ClientID != "cid_1" || got.ClientSecret != "secret_1" || got.PartnerID != "42" {
		t.Fatalf("request body = %+v", got)
	}
	if block.AccessToken != "app_at_xyz" || block.ClientID != "cid_1" ||
		block.PartnerID != "42" || block.ATExpiresAt != "2030-01-01T00:00:00Z" {
		t.Fatalf("block = %+v", block)
	}
}
