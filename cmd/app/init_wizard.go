package appcmd

import (
	"context"
	"errors"
	"slices"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
)

// createNewApp is the app picker's create-new sentinel; a NUL byte cannot
// collide with a real client_id.
const createNewApp = "\x00create"

// Hints shown when a picker's list fails to load; each names its own step's flag first.
const (
	partnerListHint = "pass --partner <partner_id> to name one directly, or --client-id <id> to link an app instead"
	appListHint     = "pass --client-id <id> to link an app directly, or --name <name> to create one"
)

// nameColWidth caps the first column of the two-column pickers.
const nameColWidth = 32

// wizardInit asks the screens stepsFor selects and returns the flags as if the
// answers had been typed on the command line. root is the directory the project
// sub-dir is created in; the name screen validates against it.
//
// Two forms: huh's Select.OptionsFunc drops its first option, so the app list uses static Options().
func wizardInit(ctx context.Context, d *app.Dashboard, root string, fl initFlags) (initFlags, error) {
	// Link mode asks nothing and reads no list.
	if fl.clientID != "" {
		return fl, nil
	}

	// Auto-select the sole partner BEFORE stepsFor, or --name alone would be planned a prompt.
	partners, err := narrowPartner(ctx, d, &fl)
	if err != nil {
		return fl, err
	}

	steps := stepsFor(fl)
	if len(steps) == 0 {
		return fl, nil
	}

	// Screen 1 — the partner, in its own form.
	if slices.Contains(steps, stepPartner) {
		if err := interact.Run(func() *huh.Form {
			return interact.NewForm(huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which partner?").
					Description("The app belongs to this partner.").
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

	// The app list for the partner now settled.
	apps, err := d.GetApps(ctx, fl.partner)
	if err != nil {
		return fl, apiError(err).WithHint(appListHint)
	}

	choice, name := createNewApp, ""
	if err := interact.Run(func() *huh.Form {
		return interact.NewForm(
			// Screen 2 — the first row creates, any other row links.
			huh.NewGroup(
				huh.NewSelect[string]().
					Title("Which app?").
					Description("Pick an existing app to link, or create a new one.").
					Options(appOptions(apps.Apps)...).
					Value(&choice),
			),
			// Screen 3 — only on the create branch.
			huh.NewGroup(
				huh.NewInput().
					Title("What is the app called?").
					Description("A project directory is created from this name.").
					Placeholder("My App").
					// OnlyOnSubmit: Input.Blur validates too, and huh refuses to
					// leave a group holding an error — esc would be swallowed.
					Validate(interact.OnlyOnSubmit(validateAppName(root))).
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
	// Link mode: the partner comes from the app, and clearing it keeps runInit
	// from warning that --partner was ignored.
	fl.clientID, fl.name, fl.partner = choice, "", ""
	return fl, nil
}

// narrowPartner fills in fl.partner where no prompt is needed — an explicit flag,
// or the only partner there is — and returns the list the picker needs otherwise.
// The list is nil when fl.partner was already set.
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
		return nil, output.ErrValidation("no partners available for this account")
	case 1:
		fl.partner = string(resp.Partners[0].ID)
	}
	return resp.Partners, nil
}

// partnerOptions lays partners out as "business name  id", falling back to the
// id alone when business_name is empty.
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
			label = pad(names[i], w) + " " + id
		}
		out = append(out, huh.NewOption(label, id))
	}
	return out
}

// appOptions puts the create sentinel first, then the partner's apps as two
// columns (name, client_id).
func appOptions(apps []app.App) []huh.Option[string] {
	names := make([]string, len(apps))
	for i, a := range apps {
		names[i] = a.Name
	}
	w := colWidth(names)
	out := make([]huh.Option[string], 0, len(apps)+1)
	out = append(out, huh.NewOption("Create a new app…", createNewApp))
	for i, a := range apps {
		out = append(out, huh.NewOption(pad(names[i], w)+" "+a.ClientID, a.ClientID))
	}
	return out
}

// colWidth returns the first-column width in terminal cells: the widest name,
// capped at nameColWidth.
func colWidth(names []string) int {
	w := 0
	for _, n := range names {
		if c := lipgloss.Width(n); c > w {
			w = c
		}
	}
	return min(w, nameColWidth)
}

// pad right-pads s to w terminal cells. fmt's %-*s pads by rune count, which
// leaves a CJK name one cell short per character and skews the second column.
func pad(s string, w int) string {
	if n := w - lipgloss.Width(s); n > 0 {
		return s + strings.Repeat(" ", n)
	}
	return s
}

// validateAppName rejects a blank name, or one whose slug already exists as a
// directory. It defers to targetDirFor, the same check runInit runs.
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
