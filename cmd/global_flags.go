package cmd

import "github.com/spf13/pflag"

// RegisterGlobalFlags wires shared flags onto the provided flag set.
//
// --dry-run and --jq live on the trees that honor them, not here, so they
// don't surface as inert global flags under commands that ignore them.
func RegisterGlobalFlags(flags *pflag.FlagSet) {
	flags.String("format", "json", `Output format: json (default), pretty, table`)
	flags.String("profile", "", "Profile to use for this invocation (overrides SHOPLAZZA_CLI_PROFILE and the current profile)")
}
