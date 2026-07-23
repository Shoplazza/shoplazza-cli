package profile

import (
	"strings"
	"testing"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/internal/core"
	"github.com/Shoplazza/shoplazza-cli/internal/keychain"
)

func TestUse_SwitchAndPrevious(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // current=us
	runCmd(t, f, "use", "--name", "cn")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.CurrentProfile != "cn" || cfg.PreviousProfile != "us" {
		t.Fatalf("%+v", cfg)
	}
	runCmd(t, f, "use", "--previous")
	cfg, _ = core.LoadConfig(f.ConfigPath)
	if cfg.CurrentProfile != "us" || cfg.PreviousProfile != "cn" {
		t.Fatalf("toggle: %+v", cfg)
	}
}

func TestUse_PreviousEmpty_Errors(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // previous is empty
	err := runCmdErr(t, f, "use", "--previous")
	if err == nil || !strings.Contains(err.Error(), "no previous profile") {
		t.Fatalf("want friendly error, got %v", err)
	}
}

func TestUpdate_ScopeChange_ClearsToken(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn")
	seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-old", time.Now().Add(time.Hour))
	runCmd(t, f, "update", "--name", "us", "--scope", "read_product")
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "" {
		t.Fatalf("old AT must be cleared, got v=%q err=%v", v, err)
	}
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if got := cfg.FindProfile("us").Scopes; len(got) != 1 || got[0] != "read_product" {
		t.Fatalf("scopes: %v", got)
	}
	if m, _ := internalauth.LoadProfileMeta(internalauth.AuthDir(f.ConfigPath), "us"); m.ExpiresAt != "" {
		t.Fatalf("old meta must be cleared, got %+v", m)
	}
}

func TestRemove_CurrentAndPointers(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // current=us
	setPreviousProfile(t, f, "cn")      // previous=cn, fixture set up explicitly
	runCmd(t, f, "remove", "--name", "us")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("us") != nil || cfg.CurrentProfile != "cn" {
		t.Fatalf("%+v", cfg) // auto-switches to first remaining
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("us")); err != nil || v != "" {
		t.Fatalf("keychain must be cleaned, got v=%q err=%v", v, err)
	}
	// Account-level credentials are untouched: uat survives.
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com")); err != nil || v != "uat-1" {
		t.Fatalf("account credentials must survive profile removal, got v=%q err=%v", v, err)
	}
}

// CMD-09: --name and --previous are mutually exclusive; CMD-13: remove of an
// unknown name errors.
func TestUse_FlagConflictAndRemoveMissing(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn")
	if err := runCmdErr(t, f, "use", "--name", "us", "--previous"); err == nil {
		t.Fatal("mutually exclusive flags must error")
	}
	if err := runCmdErr(t, f, "remove", "--name", "ghost"); err == nil ||
		!strings.Contains(err.Error(), "not found") {
		t.Fatalf("remove missing profile: %v", err)
	}
}

func TestRename_MovesEverything(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // current=us
	seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-1", time.Now().Add(time.Hour))
	runCmd(t, f, "rename", "--from", "us", "--to", "prod-us")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("prod-us") == nil || cfg.CurrentProfile != "prod-us" {
		t.Fatalf("%+v", cfg)
	}
	if v, _ := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("prod-us")); v != "at-1" {
		t.Fatal("keychain entry must move")
	}
	if m, _ := internalauth.LoadProfileMeta(internalauth.AuthDir(f.ConfigPath), "us"); m.ExpiresAt != "" {
		t.Fatal("old meta must be gone")
	}
}

// Case-only renames must not trip the "already exists" duplicate check
// against themselves (FindProfile is case-insensitive).
func TestRename_CaseOnly_Allowed(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn")
	runCmd(t, f, "rename", "--from", "us", "--to", "US")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("US").Name != "US" {
		t.Fatalf("%+v", cfg)
	}
}

// Case-only renames must preserve the cached token: since ProfileStoreKey
// and the meta path both lowercase the name, oldKey == newKey here, so a
// blind Set-then-Remove would delete what was just written.
func TestRename_CaseOnly_PreservesToken(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn")
	seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-1", time.Now().Add(time.Hour))
	runCmd(t, f, "rename", "--from", "us", "--to", "US")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("US").Name != "US" {
		t.Fatalf("%+v", cfg)
	}
	if v, err := keychain.Get(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey("US")); err != nil || v != "at-1" {
		t.Fatalf("token must survive case-only rename, got v=%q err=%v", v, err)
	}
}

func TestRename_ToExistingDifferentProfile_Errors(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn")
	err := runCmdErr(t, f, "rename", "--from", "us", "--to", "cn")
	if err == nil || !strings.Contains(err.Error(), "already exists") {
		t.Fatalf("want already-exists error, got %v", err)
	}
}

func TestRename_DefaultsToCurrent(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // current=us
	runCmd(t, f, "rename", "--to", "prod-us")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if cfg.FindProfile("prod-us") == nil || cfg.CurrentProfile != "prod-us" {
		t.Fatalf("rename without --from must target the current profile: %+v", cfg)
	}
}

func TestUpdate_DefaultsToCurrent(t *testing.T) {
	f := seedTwoProfiles(t, "us", "cn") // current=us
	runCmd(t, f, "update", "--scope", "read_product")
	cfg, _ := core.LoadConfig(f.ConfigPath)
	if got := cfg.FindProfile("us").Scopes; len(got) != 1 || got[0] != "read_product" {
		t.Fatalf("update without --name must target the current profile, scopes: %v", got)
	}
}

func TestRename_NoProfiles_Hint(t *testing.T) {
	f := newTestFactory(t, "")
	err := runCmdErr(t, f, "rename", "--to", "x")
	if !strings.Contains(err.Error(), "no profiles configured") {
		t.Fatalf("want no-profiles hint, got %v", err)
	}
}
