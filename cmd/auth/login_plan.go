package auth

// loginStep names one screen of the `auth login` wizard. Every step has exactly
// one equivalent flag; given on the command line, the step is skipped whole —
// never shown pre-filled for confirmation.
type loginStep string

// stepDomains is the business-domain multi-select. Flag: --domain (--scope
// answers the same question in the other vocabulary, so it suppresses it too).
const stepDomains loginStep = "domains"

// loginFlags is the flag state that decides which screens get asked. uat covers
// both --uat and SHOPLAZZA_UAT: same caller, same fast path.
type loginFlags struct {
	storeDomain string
	domain      []string
	scope       []string
	uat         string
	mergeScopes bool
}

// plan returns the screens this run will ask, in order. Pure: it touches no
// terminal and sends no request, so the suppression rules are directly
// testable — at the command seam a screen skipped by its flag and a screen
// skipped by a closed gate are indistinguishable.
//
// The wizard calls this to decide what to build, so there is one derivation of
// the decision rather than one here and another in each screen's hide func.
func plan(fl loginFlags, gateOpen bool) []loginStep {
	// No human present (pipe / CI / agent, or the escape hatch): ask nothing,
	// ever. This is the core invariant — the non-interactive path must stay
	// byte-for-byte what it was.
	if !gateOpen {
		return nil
	}
	// --uat (and its env form) is the documented non-interactive fast path for
	// CI: the caller already holds a token and never sees the browser flow, so
	// a picker on that path has nothing to offer. Not step-level suppression —
	// --uat is no screen's equivalent flag — but a whole-command switch, and
	// deliberately the only one besides the gate.
	if fl.uat != "" {
		return nil
	}

	var steps []loginStep
	// Permissions are asked as domains only. --scope states the answer in
	// scopes, which is just as complete, so either flag closes the screen.
	if len(fl.domain) == 0 && len(fl.scope) == 0 {
		steps = append(steps, stepDomains)
	}
	return steps
}
