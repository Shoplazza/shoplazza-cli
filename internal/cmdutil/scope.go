package cmdutil

import (
	"fmt"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// ValidateScopeSubset errors when want ⊄ granted (case-sensitive scope names).
func ValidateScopeSubset(want, granted []string) error {
	set := make(map[string]struct{}, len(granted))
	for _, s := range granted {
		set[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := set[s]; !ok {
			return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				fmt.Sprintf("scope %q is not granted to this account", s),
				"re-run 'shoplazza auth login' with the scopes you need (see 'shoplazza auth scopes')")
		}
	}
	return nil
}
