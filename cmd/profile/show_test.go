package profile

import (
	"encoding/json"
	"testing"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/core"
)

func TestProfileShow_DefaultsToCurrent_TokenAbsent(t *testing.T) {
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: "us", Account: "alice@co.com", StoreDomain: "us.myshoplazza.com", StoreID: "1"},
	}
	f.Config.CurrentProfile = "us"

	out := runCmd(t, f, "show")

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
}

func TestProfileShow_ByName_ExpiredToken(t *testing.T) {
	f := newTestFactory(t, "")
	f.Config.Profiles = []core.ProfileConfig{
		{Name: "cn", Account: "alice@co.com", StoreDomain: "cn.myshoplazza.com"},
	}
	f.Config.CurrentProfile = "us" // not the requested one

	authDir := internalauth.AuthDir(f.ConfigPath)
	if err := internalauth.SaveProfileMeta(authDir, "cn", internalauth.ProfileMeta{
		StoreID: "2", ExpiresAt: time.Now().Add(-time.Hour).Format(time.RFC3339),
	}); err != nil {
		t.Fatalf("seed meta: %v", err)
	}

	out := runCmd(t, f, "show", "--name", "cn")

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
}

func TestProfileShow_NoCurrentNoName_Errors(t *testing.T) {
	f := newTestFactory(t, "")
	if err := runCmdErr(t, f, "show"); err == nil {
		t.Fatal("expected an error when no current profile and no --name")
	}
}

func TestProfileShow_UnknownName_Errors(t *testing.T) {
	f := newTestFactory(t, "")
	if err := runCmdErr(t, f, "show", "--name", "ghost"); err == nil {
		t.Fatal("expected an error for unknown profile name")
	}
}
