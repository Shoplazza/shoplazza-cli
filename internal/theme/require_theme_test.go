package theme

import (
	"errors"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

func TestRequireThemeID_NonEmptyPassesThrough(t *testing.T) {
	got, err := RequireThemeID("abc123")
	if err != nil {
		t.Fatal(err)
	}
	if got != "abc123" {
		t.Errorf("got %q, want abc123", got)
	}
}

func TestRequireThemeID_EmptyReturnsMissingFlagError(t *testing.T) {
	_, err := RequireThemeID("")
	if !errors.Is(err, ErrMissingThemeFlag) {
		t.Fatalf("expected ErrMissingThemeFlag, got %v", err)
	}
}

func TestRequireThemeID_ErrorEnvelopeShape(t *testing.T) {
	_, err := RequireThemeID("")
	env := extractEnvelope(t, err)
	if env["type"] != "validation" || env["code"] != 2 {
		t.Errorf("envelope: %v", env)
	}
	if !strings.Contains(env["message"].(string), "missing required flag --theme-id") {
		t.Errorf("message: %v", env["message"])
	}
	hint, _ := env["hint"].(string)
	if !strings.Contains(hint, "shoplazza themes list") {
		t.Errorf("hint must reference `shoplazza themes list`: %v", hint)
	}
}

func TestMissingThemeFlagError_ErrorAndUnwrap(t *testing.T) {
	_, err := RequireThemeID("")
	// Error() delegates to the embedded ExitError's message.
	if err.Error() == "" {
		t.Error("Error() should return a non-empty message")
	}
	// Unwrap() surfaces the *output.ExitError for errors.As.
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Error("errors.As should find *output.ExitError via Unwrap()")
	}
}

// ── ValidateThemeID ───────────────────────────────────────────────────────────

func TestValidateThemeID_AcceptsSafeIDs(t *testing.T) {
	for _, id := range []string{"abc123", "ABC-def_9", "1", "_", "-"} {
		if err := ValidateThemeID(id); err != nil {
			t.Errorf("ValidateThemeID(%q) = %v, want nil", id, err)
		}
	}
	// Empty is allowed — optionality is the caller's concern.
	if err := ValidateThemeID(""); err != nil {
		t.Errorf("ValidateThemeID(\"\") = %v, want nil", err)
	}
}

func TestValidateThemeID_RejectsUnsafeIDs(t *testing.T) {
	for _, id := range []string{"../x", "a/b", `a\b`, "a b", "a.b", "a?b=c", "a#b", "%2e%2e", "id\n"} {
		err := ValidateThemeID(id)
		if err == nil {
			t.Errorf("ValidateThemeID(%q) = nil, want validation error", id)
			continue
		}
		env := extractEnvelope(t, err)
		if env["type"] != "validation" || env["code"] != 2 {
			t.Errorf("ValidateThemeID(%q) envelope: %v", id, env)
		}
	}
}

// TestRequireThemeID_ValidatesCharset: the required-flag path shares the same
// validator — junk that would rewrite URL paths is rejected.
func TestRequireThemeID_ValidatesCharset(t *testing.T) {
	_, err := RequireThemeID("../../etc")
	if err == nil {
		t.Fatal("expected validation error for malformed theme id")
	}
	env := extractEnvelope(t, err)
	if env["type"] != "validation" {
		t.Errorf("type: %v", env["type"])
	}
}
