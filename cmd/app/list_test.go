package appcmd

import (
	"bytes"
	"context"
	"encoding/json"
	"net/http"
	"strings"
	"testing"
)

// TestRunList_FieldsAndTotal locks the per-app fields (partner, id, client_id,
// name) and the envelope total.
func TestRunList_FieldsAndTotal(t *testing.T) {
	d := dashFor(t, func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/partners"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"partners": []map[string]any{{"id": "3665"}}}})
		case strings.HasSuffix(r.URL.Path, "/partners/3665/apps"):
			_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success",
				"data": map[string]any{"total": 2, "apps": []map[string]any{
					{"id": 166145, "client_id": "cid_a", "name": "app00"},
					{"id": 166143, "client_id": "cid_b", "name": "app99"},
				}}})
		default:
			t.Fatalf("unexpected path %s", r.URL.Path)
		}
	})

	var buf bytes.Buffer
	if err := runList(context.Background(), d, "3665", &buf, "json", ""); err != nil {
		t.Fatalf("runList: %v", err)
	}
	var out struct {
		Data struct {
			Total int `json:"total"`
			Apps  []struct {
				Partner  string `json:"partner"`
				ID       string `json:"id"`
				ClientID string `json:"client_id"`
				Name     string `json:"name"`
			} `json:"apps"`
		} `json:"data"`
	}
	if err := json.Unmarshal(buf.Bytes(), &out); err != nil {
		t.Fatalf("unmarshal: %v", err)
	}
	if out.Data.Total != 2 || len(out.Data.Apps) != 2 {
		t.Fatalf("total=%d apps=%d, want 2/2", out.Data.Total, len(out.Data.Apps))
	}
	a := out.Data.Apps[0]
	if a.Partner != "3665" || a.ID != "166145" || a.ClientID != "cid_a" || a.Name != "app00" {
		t.Fatalf("first app = %+v", a)
	}
}
