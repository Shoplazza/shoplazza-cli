package auth_test

import (
	"strings"
	"testing"

	internalauth "shoplazza-cli-v2/internal/auth"
)

// Pure scope-vocabulary functions: SupportedScopes / ValidateScopes.
// Manager-level scope behaviour (AvailableScopes, granted scopes after
// login) lives in manager_extra_test.go.

func TestSupportedScopes_NotEmpty(t *testing.T) {
	scopes := internalauth.SupportedScopes()
	if len(scopes) == 0 {
		t.Error("SupportedScopes() should return at least one scope")
	}
}

func TestSupportedScopes_IsSorted(t *testing.T) {
	scopes := internalauth.SupportedScopes()
	for i := 1; i < len(scopes); i++ {
		if scopes[i] < scopes[i-1] {
			t.Errorf("SupportedScopes not sorted: %q before %q", scopes[i-1], scopes[i])
		}
	}
}

func TestValidateScopes_ValidScopes(t *testing.T) {
	// Use the first supported scope — guaranteed to exist.
	scopes := internalauth.SupportedScopes()
	if len(scopes) == 0 {
		t.Skip("no supported scopes")
	}
	if err := internalauth.ValidateScopes([]string{scopes[0]}); err != nil {
		t.Errorf("ValidateScopes(%q): %v", scopes[0], err)
	}
}

func TestValidateScopes_InvalidScope(t *testing.T) {
	err := internalauth.ValidateScopes([]string{"read_products", "xyz_invalid_scope_123"})
	if err == nil {
		t.Error("expected error for unknown scope")
	}
	if !strings.Contains(err.Error(), "xyz_invalid_scope_123") {
		t.Errorf("error should mention invalid scope, got: %v", err)
	}
}

func TestValidateScopes_EmptyList(t *testing.T) {
	if err := internalauth.ValidateScopes(nil); err != nil {
		t.Errorf("empty scopes should be valid: %v", err)
	}
}
