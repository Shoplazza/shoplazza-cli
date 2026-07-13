package profile

import (
	"encoding/json"
	"testing"

	"shoplazza-cli-v2/internal/core"
)

func TestProfileList_AllProfilesWithCurrentFlag(t *testing.T) {
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com", StoreID: "1", Scopes: []string{"read_product"}},
		{Name: "cn", Account: "alice@co.com", StoreDomain: "cn.myshoplazza.com", StoreID: "2"},
	}
	f.Config.CurrentProfile = "us"

	out := runCmd(t, f, "list")

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output not JSON array: %v\n%s", err, out)
	}
	if len(items) != 2 {
		t.Fatalf("want 2 items, got %d: %s", len(items), out)
	}
	if items[0]["name"] != "us" || items[0]["current"] != true {
		t.Errorf("us entry: %+v", items[0])
	}
	if items[1]["name"] != "cn" || items[1]["current"] != false {
		t.Errorf("cn entry: %+v", items[1])
	}
	if items[0]["storeId"] != "1" || items[0]["storeDomain"] != "us.myshoplazza.com" {
		t.Errorf("us entry missing fields: %+v", items[0])
	}
	// us narrows to read_product; cn inherits the account's full granted set.
	if s, _ := items[0]["scopes"].([]any); len(s) != 1 || s[0] != "read_product" {
		t.Errorf("us scopes = %v, want [read_product] (explicit narrowing)", items[0]["scopes"])
	}
	if s, _ := items[1]["scopes"].([]any); len(s) != 2 || s[0] != "read_product" || s[1] != "write_product" {
		t.Errorf("cn scopes = %v, want account's full [read_product write_product]", items[1]["scopes"])
	}
}

func TestProfileList_Empty(t *testing.T) {
	f := newTestFactory(t, "")

	out := runCmd(t, f, "list")

	var items []map[string]any
	if err := json.Unmarshal([]byte(out), &items); err != nil {
		t.Fatalf("output not JSON array: %v\n%s", err, out)
	}
	if len(items) != 0 {
		t.Fatalf("want empty list, got %+v", items)
	}
}
