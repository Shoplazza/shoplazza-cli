package auth

import (
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/testenv"
)

// A store-scoped or --uat login of the SAME account carries no partner token;
// it must NOT wipe the existing one (that used to force a re-login for every
// app command). An account switch still clears it.
func TestPersistState_PreservesPartnerAcrossSameAccountLogin(t *testing.T) {
	testenv.IsolateConfigDir(t)
	dir := t.TempDir()
	m := &Manager{
		Config:     core.CliConfig{},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthPath:   filepath.Join(dir, "auth.json"),
	}

	// 1. Interactive login mints a partner token for account A.
	if err := m.persistState(AuthState{
		Account: "a@x.com", UAT: "uat_a",
		Partner: "ptok_a", PartnerExpiresAt: "2099-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}

	// 2. A routine store-scoped login of the SAME account returns no partner token.
	if err := m.persistState(AuthState{
		Account: "a@x.com", UAT: "uat_a", CurrentStore: "s.myshoplazza.com",
		Stores: map[string]StoreState{"s.myshoplazza.com": {Token: "stok", ExpiresAt: "2099-01-01T00:00:00Z"}},
	}); err != nil {
		t.Fatal(err)
	}
	if got, _ := keychain.Get(keychain.ShoplazzaCliService, kcPartner); got != "ptok_a" {
		t.Fatalf("partner token wiped by same-account store login: got %q, want ptok_a", got)
	}

	// 3. Switching to a DIFFERENT account with no partner token clears it.
	if err := m.persistState(AuthState{Account: "b@y.com", UAT: "uat_b"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := keychain.Get(keychain.ShoplazzaCliService, kcPartner); got != "" {
		t.Fatalf("partner token should be cleared on account switch, got %q", got)
	}
}

// The interactive and --uat login paths read the account from different backend
// endpoints (poll vs. Me), which may echo the same email with different casing.
// Preservation must match accounts case-insensitively, else a casing mismatch
// silently wipes a still-valid partner token — the exact regression this guards.
func TestPersistState_PreservesPartnerAcrossAccountCasing(t *testing.T) {
	testenv.IsolateConfigDir(t)
	dir := t.TempDir()
	m := &Manager{
		Config:     core.CliConfig{},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthPath:   filepath.Join(dir, "auth.json"),
	}

	if err := m.persistState(AuthState{
		Account: "User@X.com", UAT: "uat_a",
		Partner: "ptok_a", PartnerExpiresAt: "2099-01-01T00:00:00Z",
	}); err != nil {
		t.Fatal(err)
	}
	// Same human, but the Me endpoint returns the email lower-cased.
	if err := m.persistState(AuthState{Account: "user@x.com", UAT: "uat_a"}); err != nil {
		t.Fatal(err)
	}
	if got, _ := keychain.Get(keychain.ShoplazzaCliService, kcPartner); got != "ptok_a" {
		t.Fatalf("partner token wiped by account casing difference: got %q, want ptok_a", got)
	}
}
