package cmdutil

import (
	"os"

	"github.com/Shoplazza/shoplazza-cli/internal/core"
	"github.com/Shoplazza/shoplazza-cli/internal/output"

	"github.com/spf13/cobra"
)

// ResolveProfile implements the four-level profile resolution. First hit
// wins: --profile flag > SHOPLAZZA_CLI_PROFILE > config.CurrentProfile. An
// unknown name at any level is a loud error — it never falls through to the
// next level (silently picking a different profile than the one the user
// named would be worse than failing).
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
	if f.Config.CurrentProfile != "" {
		return lookup(f.Config.CurrentProfile, "current profile")
	}
	return nil, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no profile configured",
		"run 'shoplazza auth login -s <store>' or 'shoplazza profile add' to create one")
}
