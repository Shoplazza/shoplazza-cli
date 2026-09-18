package cmdutil

import (
	"os"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/spf13/cobra"
)

// ResolveProfile implements the profile resolution chain. First hit wins:
// --profile flag > SHOPLAZZA_CLI_PROFILE > theme environment (-e, theme commands
// only) > config.CurrentProfile. An unknown name at any level is a loud error —
// it never falls through to the next level (silently picking a different profile
// than the one the user named would be worse than failing).
func ResolveProfile(f *Factory, cmd *cobra.Command) (*core.ProfileConfig, error) {
	lookup := func(name, source string) (*core.ProfileConfig, error) {
		if p := f.Config.FindProfile(name); p != nil {
			return p, nil
		}
		return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"profile "+name+" not found ("+source+")",
			"run 'shoplazza profile list' to see profiles, or 'shoplazza profile add' to create one")
	}
	if cmd != nil {
		if v, _ := cmd.Flags().GetString("profile"); v != "" {
			return lookup(v, "--profile flag")
		}
	}
	if v := os.Getenv("SHOPLAZZA_CLI_PROFILE"); v != "" {
		return lookup(v, "SHOPLAZZA_CLI_PROFILE")
	}
	// A theme -e/SHOPLAZZA_CLI_ENVIRONMENT selection picks the store/profile next,
	// below an explicit profile but above the ambient current one. Inert unless
	// the command defines --environment (theme commands only).
	if p, ok, err := profileFromThemeEnv(f, cmd); err != nil {
		return nil, err
	} else if ok {
		return p, nil
	}
	if f.Config.CurrentProfile != "" {
		return lookup(f.Config.CurrentProfile, "current profile")
	}
	return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no profile configured",
		"run 'shoplazza auth login -s <store>' or 'shoplazza profile add' to create one")
}
