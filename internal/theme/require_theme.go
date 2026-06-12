package theme

import (
	"errors"
	"regexp"

	"shoplazza-cli-v2/internal/output"
)

// ErrMissingThemeFlag is the sentinel returned (wrapped) by RequireThemeID
// when the caller did not supply --theme-id. Callers can detect this with
// errors.Is(err, theme.ErrMissingThemeFlag) without depending on the
// concrete wrapping error type.
var ErrMissingThemeFlag = errors.New("missing required flag --theme-id")

// missingThemeFlagHint is the operator-facing guidance attached to the
// envelope when --theme-id is missing. It points at `shoplazza themes list`
// — the dynamic CRUD command that enumerates theme IDs.
const missingThemeFlagHint = `To find available theme IDs, run:
   shoplazza themes list

Then re-run with --theme-id <theme_id>, e.g.:
   shoplazza themes pull --theme-id <theme_id>`

// missingThemeFlagError wraps an *output.ExitError so callers can match
// the ErrMissingThemeFlag sentinel via errors.Is, assert the Envelope()
// interface on the outer error, and reach the envelope carrier via Unwrap.
type missingThemeFlagError struct {
	envErr *output.ExitError
}

func (e *missingThemeFlagError) Error() string { return e.envErr.Error() }

// Unwrap exposes the embedded *output.ExitError so generic envelope
// extractors (errors.As) can reach the structured detail.
func (e *missingThemeFlagError) Unwrap() error { return e.envErr }

// Is matches the package-level sentinel so callers can pattern-match
// without importing the concrete error type.
func (e *missingThemeFlagError) Is(target error) bool {
	return target == ErrMissingThemeFlag
}

// Envelope delegates to the wrapped *output.ExitError so the outer error
// itself satisfies the `interface{ Envelope() map[string]any }` shape
// required by cmd/root.go and test helpers.
func (e *missingThemeFlagError) Envelope() map[string]any {
	return e.envErr.Envelope()
}

// RequireThemeID enforces the mandatory --theme-id flag with no
// interactive fallback. When themeFlag is non-empty it is returned
// verbatim; otherwise a validation-class error is produced that wraps
// the ErrMissingThemeFlag sentinel and carries the operator-facing hint
// pointing at `shoplazza themes list`.
//
// This helper is deliberately stdlib-only (plus internal/output) so an
// import-guard test can prove no TTY / prompt dependency
// (golang.org/x/term, survey, promptui, bubbletea, huh) leaks in via
// this path.
func RequireThemeID(themeFlag string) (string, error) {
	if themeFlag != "" {
		if err := ValidateThemeID(themeFlag); err != nil {
			return "", err
		}
		return themeFlag, nil
	}
	env := output.ErrWithHint(
		output.ExitValidation,
		output.TypeValidation,
		"missing required flag --theme-id",
		missingThemeFlagHint,
	)
	return "", &missingThemeFlagError{envErr: env}
}

// themeIDPattern is the charset a theme id may use. Theme ids are spliced
// into URL paths (service.go plan factories) and tmp-zip filenames
// (pull.go tempZipPath), so anything outside this set — slashes, dots,
// spaces, control bytes — is rejected up front rather than allowed to
// rewrite the request path or escape the tmp directory.
var themeIDPattern = regexp.MustCompile(`^[A-Za-z0-9_-]+$`)

// ValidateThemeID rejects --theme-id values containing characters outside
// [A-Za-z0-9_-]. Empty input is allowed — optional-flag callers (serve,
// share) handle absence themselves; required-flag callers go through
// RequireThemeID which validates after the presence check.
func ValidateThemeID(id string) error {
	if id == "" {
		return nil
	}
	if !themeIDPattern.MatchString(id) {
		return output.ErrValidation(
			"invalid --theme-id %q: only letters, digits, '-' and '_' are allowed", id)
	}
	return nil
}
