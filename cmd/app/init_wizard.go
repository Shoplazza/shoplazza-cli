package appcmd

import (
	"context"
	"errors"
	"fmt"
	"slices"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/huh"
)

// createNewApp is the app picker's sentinel for "none of the above". A NUL byte
// cannot collide with a real client_id.
const createNewApp = "\x00create"

// A list that will not load is reported, never worked around: every step has an
// equivalent flag, so the hint has somewhere to point and a fallback input box
// would only re-do an existing flag with a worse UI. Each hint names its own
// step's flag first.
const (
	partnerListHint = "pass --partner <partner_id> to name one directly, or --client-id <id> to link an app instead"
	appListHint     = "pass --client-id <id> to link an app directly, or --name <name> to create one"
)

// nameColWidth bounds the first column of the two-column pickers so one long
// name cannot push every client_id off a narrow terminal.
const nameColWidth = 32

// wizardInit asks the screens plan() selected and returns the flags as if the
// answers had been typed on the command line, so runInit sees no difference.
//
// It is deliberately TWO forms. huh v1.0.0's Select.OptionsFunc — the obvious
// way to re-fetch apps when the partner changes — drops its first option from
// the list as soon as the cursor leaves it, so a list that depends on an
// upstream answer cannot live in the same form. The partner is answered in its
// own small form, its apps are fetched ONCE, and the main form gets static
// Options(). The cost is that esc cannot cross back into the partner screen;
// accepted (see docs/solutions/interactive-cli/03-app-init.md).
//
// root is the directory the project sub-dir is created in — the name screen
// validates against it.
func wizardInit(ctx context.Context, d *app.Dashboard, root string, fl initFlags) (initFlags, error) {
	// Link mode asks nothing, and returning here keeps `--client-id` free of the
	// reads the pickers need: zero interaction AND zero extra requests.
	if fl.clientID != "" {
		return fl, nil
	}

	// Auto-select the sole partner BEFORE planning, exactly as selectPartner does
	// non-interactively. Left until after, a single-partner account running
	// `app init --name X` would be planned a partner prompt it has no choice in.
	partners, err := narrowPartner(ctx, d, &fl)
	if err != nil {
		return fl, err
	}

	steps := stepsFor(fl)
	if len(steps) == 0 {
		return fl, nil
	}

	// Screen 1 — the partner. Its own form; see the OptionsFunc note above.
	if slices.Contains(steps, stepPartner) {
		if err := interact.Run(func() *huh.Form {
			return interact.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which partner?").
					Description("The app is created under, or looked up in, this partner.").
					Options(partnerOptions(partners)...).
					Value(&fl.partner),
			))
		}); err != nil {
			return fl, err
		}
	}
	if !slices.Contains(steps, stepApp) {
		return fl, nil // --name answered the rest
	}

	// The one extra read the pickers need, for the partner now settled. A single
	// fast request between two forms — no progress indicator.
	apps, err := d.GetApps(ctx, fl.partner)
	if err != nil {
		return fl, apiError(err).WithHint(appListHint)
	}

	// Not seeded from fl.name: reaching here means --name was absent, and a
	// given flag is never pre-filled for confirmation anyway.
	choice, name := createNewApp, ""
	if err := interact.Run(func() *huh.Form {
		return interact.NewForm(
			// Screen 2 — the choice IS the mode: the first row creates, any other
			// row links. No separate "new or existing?" question.
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which app?").
					Description("Pick an existing app to link, or create a new one.").
					Options(appOptions(apps.Apps)...).
					Value(&choice),
			),
			// Screen 3 — only on the create branch. Answer-driven narrowing is
			// the form's job, live; plan() only reports what the flags leave open.
			huh.NewGroup(
				huh.NewInput().
					Title("What is the app called?").
					Description("A project directory is created from this name.").
					Placeholder("My App").
					Validate(validateAppName(root)).
					Value(&name),
			).WithHideFunc(func() bool { return choice != createNewApp }),
		)
	}); err != nil {
		return fl, err
	}

	if choice == createNewApp {
		fl.name = strings.TrimSpace(name)
		return fl, nil
	}
	// An existing app was picked: that is link mode, and the partner is derived
	// from the app itself. Dropping it here also keeps runInit from warning that
	// a --partner we actually USED (to filter this list) was ignored.
	fl.clientID, fl.name, fl.partner = choice, "", ""
	return fl, nil
}

// narrowPartner resolves the partner without asking where it can — explicit
// flag, or the only one there is — and returns the list the picker needs when it
// cannot. The single GetPartners call is the wizard's first request and doubles
// as the token pre-check requireLogin cannot do (it only reads local state).
func narrowPartner(ctx context.Context, d *app.Dashboard, fl *initFlags) ([]app.Partner, error) {
	if fl.partner != "" {
		return nil, nil
	}
	resp, err := d.GetPartners(ctx)
	if err != nil {
		return nil, apiError(err).WithHint(partnerListHint)
	}
	switch len(resp.Partners) {
	case 0:
		// Nothing to pick and nothing to create under — the same message
		// selectPartner gives this account non-interactively.
		return nil, output.ErrValidation("no partners available for this account")
	case 1:
		fl.partner = string(resp.Partners[0].ID)
	}
	return resp.Partners, nil
}

// partnerOptions lays partners out as "business name  id". The Dashboard can
// leave business_name empty, in which case the id carries the row alone.
func partnerOptions(partners []app.Partner) []huh.Option[string] {
	names := make([]string, len(partners))
	for i, p := range partners {
		names[i] = strings.TrimSpace(p.BusinessName)
	}
	w := colWidth(names)
	out := make([]huh.Option[string], 0, len(partners))
	for i, p := range partners {
		id := string(p.ID)
		label := id
		if names[i] != "" {
			label = fmt.Sprintf("%-*s %s", w, names[i], id)
		}
		out = append(out, huh.NewOption(label, id))
	}
	return out
}

// appOptions puts "create" first, then the resolved partner's apps as two
// columns (name, client_id). No column header — with two self-evident columns it
// would be noise — and no partner column: the list is already filtered to one,
// so that column would read the same on every row.
func appOptions(apps []app.App) []huh.Option[string] {
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	w := colWidth(names)
	out := make([]huh.Option[string], 0, len(apps)+1)
	out = append(out, huh.NewOption("Create a new app…", createNewApp))
	for i, a := range apps {
		out = append(out, huh.NewOption(fmt.Sprintf("%-*s %s", w, names[i], a.ClientID), a.ClientID))
	}
	return out
}

// colWidth pads the first column to the longest entry, capped so one outlier
// cannot push the second column off screen.
func colWidth(names []string) int {
	w := 0
	for _, n := range names {
		if len(n) > w {
			w = len(n)
		}
	}
	return min(w, nameColWidth)
}

// validateAppName rejects a name whose slug already exists as a directory —
// the failure the command otherwise only reports after creating the app
// server-side. It calls the SAME targetDirFor runInit checks with: a second copy
// of the slug-and-stat rule here would drift from the one that actually runs.
func validateAppName(root string) func(string) error {
	return func(s string) error {
		s = strings.TrimSpace(s)
		if s == "" {
			return errors.New("app name is required")
		}
		_, _, err := targetDirFor(root, s)
		return err
	}
}
