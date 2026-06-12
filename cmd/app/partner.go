package appcmd

import (
	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/output"
)

// selectPartner resolves the partner context non-interactively: explicit flag
// wins; single partner auto-selects; multiple without a flag is a validation
// error (never prompt).
func selectPartner(partners []app.Partner, flag string) (string, error) {
	if flag != "" {
		for _, p := range partners {
			if string(p.ID) == flag {
				return flag, nil
			}
		}
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"partner '"+flag+"' not found in your account",
			"run 'shoplazza app list' to see available partners")
	}
	switch len(partners) {
	case 0:
		return "", output.ErrValidation("no partners available for this account")
	case 1:
		return string(partners[0].ID), nil
	default:
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"multiple partners — specify which one",
			"pass --partner <partner_id> or run 'shoplazza app list'")
	}
}
