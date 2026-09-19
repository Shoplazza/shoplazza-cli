package profile

import (
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// pickProfile lets a human fuzzy-select a configured profile when --name is
// omitted in a terminal. There is nothing to type if no profiles exist, so it
// returns a structured error rather than prompting for a name that can't match.
// Non-interactive callers never reach it.
func pickProfile(f *cmdutil.Factory, title string) (string, error) {
	opts := configuredProfileOptions(f.Config)
	if len(opts) == 0 {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no profiles configured", "run 'shoplazza auth login -s <store>' to add one")
	}
	return interact.SelectFiltered(title, opts)
}

// configuredProfileOptions builds the profile picker's choices: one per profile,
// labeled with its store domain and the current one marked. Value is the bare
// profile name. Pure, so it is unit-tested without a terminal.
func configuredProfileOptions(cfg core.CliConfig) []interact.Option {
	opts := make([]interact.Option, 0, len(cfg.Profiles))
	for i := range cfg.Profiles {
		p := cfg.Profiles[i]
		label := p.Name
		if p.StoreDomain != "" {
			label += " — " + p.StoreDomain
		}
		if strings.EqualFold(cfg.CurrentProfile, p.Name) {
			label += " (current)"
		}
		opts = append(opts, interact.Option{Label: label, Value: p.Name})
	}
	return opts
}
