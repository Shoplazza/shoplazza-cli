package appcmd

// initStep names one screen of the `app init` wizard. Every step has exactly one
// equivalent flag; given on the command line, the step is skipped whole.
type initStep string

const (
	// stepPartner picks the owning partner, which also narrows the app list.
	// Flag: --partner.
	stepPartner initStep = "partner"

	// stepApp picks an existing app to link, or its first row to create a new
	// one. Flags: --client-id / --name.
	stepApp initStep = "app"

	// stepName names the app being created. Flag: --name.
	stepName initStep = "name"
)

// initFlags is the flag state that decides which screens get asked. partner
// holds --partner or the sole partner auto-selected before planning.
type initFlags struct {
	clientID string
	name     string
	partner  string
}

// stepsFor returns the screens the flags leave unanswered, in order. It touches
// no terminal; the interactive gate is RunE's job.
func stepsFor(fl initFlags) []initStep {
	// Link mode needs nothing: resolveAppRef derives the partner from the app.
	if fl.clientID != "" {
		return nil
	}

	var steps []initStep
	if fl.partner == "" {
		steps = append(steps, stepPartner)
	}
	// --name answers both the app list ("create a new one") and the name screen.
	if fl.name == "" {
		steps = append(steps, stepApp, stepName)
	}
	return steps
}
