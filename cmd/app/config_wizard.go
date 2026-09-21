package appcmd

import (
	"context"
	"errors"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
)

// wizardLink is `app config link`'s wrapper over the shared partner→app wizard,
// used when the human named no mode: pick a partner, then link an existing app
// or create+name a new one. It scaffolds nothing (unlike init), so a created
// app's name is validated only for non-emptiness. The answers map onto linkOpts;
// rows is the summary card (nil when nothing was asked).
func wizardLink(ctx context.Context, d *app.Dashboard, o linkOpts) (linkOpts, []string, error) {
	fl, rows, err := runAppWizard(ctx, d, initFlags{name: o.Name, partner: o.Partner}, validateLinkName)
	if err != nil {
		return o, nil, err
	}
	o.Partner = fl.partner
	if fl.clientID != "" {
		o.ClientID, o.Create, o.Name = fl.clientID, false, ""
	} else {
		o.ClientID, o.Create, o.Name = "", true, fl.name
	}
	return o, rows, nil
}

// validateLinkName guards the create-name screen for `app config link`: the name
// becomes the app name and the config filename, so only a blank is rejected —
// there is no scaffold directory to collide with (that is init's concern).
func validateLinkName(s string) error {
	if strings.TrimSpace(s) == "" {
		return errors.New("app name is required")
	}
	return nil
}
