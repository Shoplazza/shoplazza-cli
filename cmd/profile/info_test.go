package profile

import (
	"encoding/json"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/core"
)

func TestProfileInfo_DefaultsToCurrent_TokenAbsent(t *testing.T) {
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com", StoreID: "1"},
	}
	f.Config.CurrentProfile = "us"

	out := runCmd(t, f, "info")

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["name"] != "us" || got["current"] != true {
		t.Fatalf("got: %+v", got)
	}
	if got["tokenStatus"] != "absent" {
		t.Errorf("tokenStatus = %v, want absent (no meta written)", got["tokenStatus"])
	}
	// No narrowing and no minted token: scopes defaults to the account's full
	// granted set, never a bare null.
	scopes, ok := got["scopes"].([]any)
	if !ok || len(scopes) != 2 || scopes[0] != "read_product" || scopes[1] != "write_product" {
		t.Errorf("scopes = %v, want the account's full [read_product write_product]", got["scopes"])
	}
}

func TestProfileInfo_ByName_ExpiredToken_ScopesFromGrant(t *testing.T) {
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: "cn", Account: "alice@co.com", StoreDomain: "cn.myshoplazza.com"},
	}
	f.Config.CurrentProfile = "us" // not the requested one

	authDir := internalauth.AuthDir(f.ConfigPath)
	if err := internalauth.SaveProfileMeta(authDir, "cn", internalauth.ProfileMeta{
		StoreID: "2", ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
		GrantedScopes: []string{"read_product", "write_product"},
	}); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	out := runCmd(t, f, "info", "--name", "cn")

	var got map[string]any
	if err := json.Unmarshal([]byte(out), &got); err != nil {
		t.Fatalf("output not JSON: %v\n%s", err, out)
	}
	if got["current"] != false {
		t.Errorf("current = %v, want false", got["current"])
	}
	if got["tokenStatus"] != "expired" {
		t.Errorf("tokenStatus = %v, want expired", got["tokenStatus"])
	}
	if got["storeId"] != "2" {
		t.Errorf("storeId = %v, want fallback to meta's 2", got["storeId"])
	}
	// A minted token: scopes reflects the exchange's granted set from meta.
	scopes, ok := got["scopes"].([]any)
	if !ok || len(scopes) != 2 || scopes[0] != "read_product" || scopes[1] != "write_product" {
		t.Errorf("scopes = %v, want [read_product write_product] from meta", got["scopes"])
	}
}

func TestProfileInfo_NoCurrentNoName_Errors(t *testing.T) {
	f := newTestFactory(t, "")
	if err := runCmdErr(t, f, "info"); err == nil {
		t.Fatal("expected an error when no current profile and no --name")
	}
}

func TestProfileInfo_UnknownName_Errors(t *testing.T) {
	f := newTestFactory(t, "")
	if err := runCmdErr(t, f, "info", "--name", "ghost"); err == nil {
		t.Fatal("expected an error for unknown profile name")
	}
}
