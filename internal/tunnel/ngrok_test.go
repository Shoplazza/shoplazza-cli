package tunnel

import "testing"

// TestNgrokAuthTokenResolution: the Token struct field wins; an empty field
// falls back to $NGROK_AUTHTOKEN; neither set → empty (strategy unavailable).
func TestNgrokAuthTokenResolution(t *testing.T) {
	t.Setenv("NGROK_AUTHTOKEN", "from_env")
	if got := (&Ngrok{Token: "from_field"}).authToken(); got != "from_field" {
		t.Fatalf("Token field should win over env: got %q", got)
	}
	if got := (&Ngrok{}).authToken(); got != "from_env" {
		t.Fatalf("empty field should fall back to env: got %q", got)
	}
	t.Setenv("NGROK_AUTHTOKEN", "") // present but empty
	if got := (&Ngrok{}).authToken(); got != "" {
		t.Fatalf("neither set → empty: got %q", got)
	}
}

// TestNgrokDomainResolution: same precedence for the optional reserved domain.
func TestNgrokDomainResolution(t *testing.T) {
	t.Setenv("NGROK_DOMAIN", "env.ngrok.app")
	if got := (&Ngrok{Domain: "field.ngrok.app"}).reservedDomain(); got != "field.ngrok.app" {
		t.Fatalf("Domain field should win over env: got %q", got)
	}
	if got := (&Ngrok{}).reservedDomain(); got != "env.ngrok.app" {
		t.Fatalf("empty field should fall back to env: got %q", got)
	}
}
