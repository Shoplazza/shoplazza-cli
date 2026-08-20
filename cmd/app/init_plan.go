package appcmd

// initStep names one screen of the `app init` wizard. Every step has exactly one
// equivalent flag; given on the command line, the step is skipped whole — never
// shown pre-filled for confirmation.
type initStep string

const (
	// stepPartner picks the owning partner, which also narrows the app list.
	// Flag: --partner.
	stepPartner initStep = "partner"

	// stepApp is the one list that carries the mode: its first row creates a new
	// app, any other row links that existing one. Flags: --client-id / --name —
	// one mutually exclusive group, so either of them answers it.
	stepApp initStep = "app"

	// stepName names the app being created. Flag: --name.
	stepName initStep = "name"
)

// initFlags is the flag state that decides which screens get asked.
//
// partner holds --partner OR the sole partner auto-selected before plan runs: a
// single-partner account has nothing to choose, so it counts as answered. That
// resolution has to happen FIRST, or `app init --name X` on a single-partner
// account — a zero-interaction run today — would be planned a prompt.
type initFlags struct {
	clientID string
	name     string
	partner  string
}

// plan returns the screens this run will ask, in order. Pure: it touches no
// terminal and sends no request, so the suppression rules are directly testable
// — at the command seam a screen skipped by its flag and a screen skipped by a
// closed gate are indistinguishable.
//
// The wizard calls this to decide what to build, so there is one derivation of
// the decision rather than one here and another in each screen's hide func.
func plan(fl initFlags, gateOpen bool) []initStep {
	// No human present (pipe / CI / agent, or the escape hatch): ask nothing,
	// ever. Every other rule lives in stepsFor.
	if !gateOpen {
		return nil
	}
	return stepsFor(fl)
}

// stepsFor is the gate-free half: which screens the flags leave unanswered.
// The wizard calls this directly — it only ever runs with the gate open, and
// passing a literal true would make the parameter a lie.
func stepsFor(fl initFlags) []initStep {
	// Link mode is complete on its own: resolveAppRef derives the owning partner
	// FROM the app via /info, so even screen 1 has nothing to add.
	if fl.clientID != "" {
		return nil
	}

	var steps []initStep
	if fl.partner == "" {
		steps = append(steps, stepPartner)
	}
	// --name answers the app list ("create a new one") and is the name screen's
	// own flag, so it closes both at once. Whether the name screen is really
	// reached depends on the answer to stepApp — the form hides it live; plan
	// reports the steps the FLAGS leave reachable.
	if fl.name == "" {
		steps = append(steps, stepApp, stepName)
	}
	return steps
}
