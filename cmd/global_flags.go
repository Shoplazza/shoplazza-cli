package cmd

import "github.com/spf13/pflag"

// RegisterGlobalFlags wires shared flags onto the provided flag set.
//
// --dry-run and --jq live on the trees that honor them, not here, so they
// don't surface as inert global flags under commands that ignore them.
// defaultFormat is the resolved default (SHOPLAZZA_CLI_FORMAT env > "json"); an
// explicit --format on a command still overrides it.
func RegisterGlobalFlags(flags *pflag.FlagSet, defaultFormat string) {
	flags.String("format", defaultFormat, `Output format: json (default), pretty, table, ndjson, csv (env: SHOPLAZZA_CLI_FORMAT)`)
	flags.String("profile", "", "Profile to use for this invocation")
}
