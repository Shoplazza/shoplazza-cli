package auth

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"testing"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdtest"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func containsAll(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, s := range got {
		set[s] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// --domain app expands to the app-extension development scopes: themes,
// checkout, and theme-extension uploads all authorize via the themes scope.
func TestExpandLoginDomains_AppAlias(t *testing.T) {
	got, err := expandLoginDomains([]string{"app"})
	if err != nil {
		t.Fatalf("expandLoginDomains([app]): %v", err)
	}
	if !containsAll(got, "read_themes", "write_themes") {
		t.Fatalf("--domain app = %v, want read_themes + write_themes", got)
	}
}

// Non-app domains keep delegating to internalauth.ExpandDomains unchanged.
func TestExpandLoginDomains_ModuleDelegates(t *testing.T) {
	got, err := expandLoginDomains([]string{"products"})
	if err != nil {
		t.Fatalf("expandLoginDomains([products]): %v", err)
	}
	want, _ := internalauth.ExpandDomains([]string{"products"})
	if len(want) == 0 || !containsAll(got, want...) || len(got) != len(want) {
		t.Fatalf("--domain products = %v, want delegate to ExpandDomains %v", got, want)
	}
}

// app + a module domain grants the union.
func TestExpandLoginDomains_AppPlusModule(t *testing.T) {
	got, err := expandLoginDomains([]string{"app", "products"})
	if err != nil {
		t.Fatalf("expandLoginDomains([app,products]): %v", err)
	}
	prod, _ := internalauth.ExpandDomains([]string{"products"})
	want := append([]string{"read_themes", "write_themes"}, prod...)
	if !containsAll(got, want...) {
		t.Fatalf("--domain app,products = %v, want union %v", got, want)
	}
}

func TestExpandLoginDomains_Unknown(t *testing.T) {
	if _, err := expandLoginDomains([]string{"bogus"}); err == nil {
		t.Fatal("expected error for unknown domain, got nil")
	}
}

// A narrow re-login must carry the prior grant along: authorization replaces
// the account's granted set server-side, so requesting only read_inventory
// would otherwise revoke every other scope mid-task.
func TestUnionWithGranted_KeepsPriorGrant(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_inventory", "write_inventory"},
		[]string{"read_product", "write_product", "read_inventory"},
	)
	if !containsAll(got, "read_inventory", "write_inventory", "read_product", "write_product") {
		t.Fatalf("union = %v, want request ∪ grant", got)
	}
	if kept != 2 { // read_product + write_product carried over; read_inventory overlaps
		t.Fatalf("kept = %d, want 2", kept)
	}
	if got[0] != "read_inventory" || got[1] != "write_inventory" {
		t.Fatalf("union = %v, want the request's scopes first", got)
	}
}

func TestUnionWithGranted_NoPriorGrant(t *testing.T) {
	got, kept := unionWithGranted([]string{"read_shop"}, nil)
	if len(got) != 1 || got[0] != "read_shop" || kept != 0 {
		t.Fatalf("union = %v kept=%d, want unchanged request", got, kept)
	}
}

func TestUnionWithGranted_RequestSupersetOfGrant(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_product", "write_product"},
		[]string{"read_product"},
	)
	if len(got) != 2 || kept != 0 {
		t.Fatalf("union = %v kept=%d, want no carry-over when request covers grant", got, kept)
	}
}

func TestUnionWithGranted_DedupesRequest(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_shop", "read_shop"},
		[]string{"read_shop"},
	)
	if len(got) != 1 || kept != 0 {
		t.Fatalf("union = %v kept=%d, want dedup without counting overlaps as kept", got, kept)
	}
}

// runAuthCmd runs the auth command tree with args, capturing stdout, and
// fails the test on any RunE error.
func runAuthCmd(t *testing.T, f *cmdutil.Factory, args ...string) string {
	t.Helper()
	var buf bytes.Buffer
	cmd := NewCmdAuth(f)
	cmd.SetOut(&buf)
	cmd.SetErr(io.Discard)
	cmd.SetArgs(args)
	cmd.SetContext(context.Background())
	if err := cmd.Execute(); err != nil {
		t.Fatalf("auth %v: unexpected error: %v", args, err)
	}
	return buf.String()
}

// GATE-09 (display surface): auth status's tokenStatus tri-state, for the
// current profile.
func TestStatus_TokenStates(t *testing.T) {
	f := cmdtest.SeedLoggedInWithProfiles(t, "alice@co.com", "us")
	for _, tc := range []struct {
		name, wantStatus string
		expiresAt        time.Time
		seedToken        bool
	}{
		{"valid", "valid", time.Now().Add(time.Hour), true},
		{"expired", "expired", time.Now().Add(-time.Hour), true},
		{"invalid", "invalid", time.Time{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if tc.seedToken {
				seedProfileToken(t, internalauth.AuthDir(f.ConfigPath), "us", "at-x", tc.expiresAt)
			} else {
				_ = internalauth.RemoveProfileMeta(internalauth.AuthDir(f.ConfigPath), "us")
			}
			out := runAuthCmd(t, f, "status")
			var got map[string]any
			_ = json.Unmarshal([]byte(out), &got)
			rows, _ := got["profiles"].([]any)
			if len(rows) != 1 {
				t.Fatalf("profiles = %v", got["profiles"])
			}
			row, _ := rows[0].(map[string]any)
			if row["token_status"] != tc.wantStatus || row["name"] != "us" || row["current"] != true {
				t.Fatalf("profiles[0] = %v", row)
			}
		})
	}
}
