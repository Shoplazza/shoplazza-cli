package cmd

import "github.com/spf13/pflag"

// RegisterGlobalFlags wires shared flags onto the provided flag set.
// defaultFormat is the resolved default output format (SHOPLAZZA_CLI_FORMAT env
// > config `format` > "json"); an explicit --format always overrides it.
//
// --dry-run and --jq live on the trees that honor them, not here, so they
// don't surface as inert global flags under commands that ignore them.
func RegisterGlobalFlags(flags *pflag.FlagSet, defaultFormat string) {
	flags.String("format", defaultFormat, `Output format: json (machine, default), pretty (human), table, ndjson, csv`)
	flags.String("profile", "", "Profile to use for this invocation")
}
