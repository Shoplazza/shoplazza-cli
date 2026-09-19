package output

import (
	"errors"
	"testing"
)

func TestClassifyUsageError(t *testing.T) {
	tests := []struct {
		name        string
		in          string
		wantSubtype string
	}{
		{"unknown command", `unknown command "prodcuts" for "shoplazza"`, SubtypeUnknownCommand},
		{"unknown flag", "unknown flag: --bogus", SubtypeUnknownFlag},
		{"unknown shorthand", "unknown shorthand flag: 'x' in -x", SubtypeUnknownFlag},
		{"missing required", `required flag(s) "name" not set`, SubtypeMissingRequiredFlag},
		{"invalid argument", `invalid argument "abc" for "--count" flag: strconv`, SubtypeInvalidFlagValue},
		{"needs argument", "flag needs an argument: --data", SubtypeInvalidFlagValue},
		{"accepts args", "accepts 1 arg(s), received 2", SubtypeInvalidArgument},
		{"requires args", "requires at least 1 arg(s), only received 0", SubtypeInvalidArgument},
		{"generic fallback", "something else went wrong", SubtypeUsage},
	}
	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			// Arrange
			err := errors.New(tc.in)

			// Act
			got := ClassifyUsageError(err)

			// Assert
			if got.Code != ExitValidation {
				t.Errorf("Code = %d, want %d (validation)", got.Code, ExitValidation)
			}
			if got.Detail.Type != TypeValidation {
				t.Errorf("Type = %q, want %q", got.Detail.Type, TypeValidation)
			}
			if got.Detail.Subtype != tc.wantSubtype {
				t.Errorf("Subtype = %q, want %q", got.Detail.Subtype, tc.wantSubtype)
			}
			if got.Detail.Message != tc.in {
				t.Errorf("Message = %q, want %q", got.Detail.Message, tc.in)
			}
			if got.Detail.Hint == "" {
				t.Error("expected a non-empty hint")
			}
		})
	}
}

// A message with a literal % must not be treated as a format string.
func TestClassifyUsageError_PercentInMessage(t *testing.T) {
	got := ClassifyUsageError(errors.New(`invalid argument "50%" for "--rate" flag`))
	if got.Detail.Message != `invalid argument "50%" for "--rate" flag` {
		t.Errorf("message mangled: %q", got.Detail.Message)
	}
}
