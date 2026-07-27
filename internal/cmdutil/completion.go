package cmdutil

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"

	"github.com/spf13/cobra"
)

// ProfileNames lists configured profile names for shell completion.
func ProfileNames(cfg core.CliConfig) []string {
	names := make([]string, 0, len(cfg.Profiles))
	for _, p := range cfg.Profiles {
		names = append(names, p.Name)
	}
	return names
}

// ProfileNameCompletionFunc returns a cobra completion func listing f's
// configured profile names, for --profile and 'profile use --name'. Names are
// read at registration time (process start): shell completion re-execs the
// binary per keystroke, so the factory's config read is always fresh.
func ProfileNameCompletionFunc(f *Factory) func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return cobra.FixedCompletions(ProfileNames(f.Config), cobra.ShellCompDirectiveNoFileComp)
}
