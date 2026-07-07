package auth

import (
	"testing"

	internalauth "shoplazza-cli-v2/internal/auth"
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
