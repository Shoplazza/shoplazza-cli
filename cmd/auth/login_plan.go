package auth

// loginStep names one screen of the `auth login` wizard. Every step has exactly
// one equivalent flag; given on the command line, the step is skipped whole.
type loginStep string

const (
	// stepStore is the store input. Flag: -s. Blank is a real answer: it means
	// account-only login, the same as omitting -s.
	stepStore loginStep = "store"

	// stepDomains is the business-domain multi-select. Flags: --domain, --scope.
	stepDomains loginStep = "domains"
)

// loginFlags is the flag state that decides which screens get asked. uat covers
// both --uat and SHOPLAZZA_UAT.
type loginFlags struct {
	storeDomain string
	domain      []string
	scope       []string
	uat         string
	mergeScopes bool
}

// plan returns the screens this run will ask, in order. It touches no terminal
// and sends no request.
func plan(fl loginFlags, gateOpen bool) []loginStep {
	if !gateOpen {
		return nil
	}
	// --uat (including SHOPLAZZA_UAT) switches off the whole wizard, not one screen.
	if fl.uat != "" {
		return nil
	}

	var steps []loginStep
	// Order matters: the wizard builds its groups from this slice, and huh walks
	// them in build order.
	if fl.storeDomain == "" {
		steps = append(steps, stepStore)
	}
	// Either flag answers the permissions screen: --scope states it in scopes.
	if len(fl.domain) == 0 && len(fl.scope) == 0 {
		steps = append(steps, stepDomains)
	}
	return steps
}
