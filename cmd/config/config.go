// Package configcmd implements `shoplazza config`: read and edit persistent CLI
// configuration that isn't tied to a single invocation — currently the default
// output format.
package configcmd

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// NewCmdConfig builds the `config` command tree.
func NewCmdConfig(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "config",
		Short: "Read and edit persistent CLI configuration",
		Long:  "Read and edit persistent CLI configuration that outlives a single command, such as the default output format.",
	}
	cmd.AddCommand(newCmdConfigFormat(f))
	return cmd
}

// newCmdConfigFormat builds `config format [value]`: show or set the default
// output format. Setting it lets a human get readable output by default without
// passing --format every time; agents/CI (no config, or an explicit --format /
// SHOPLAZZA_CLI_FORMAT) are unaffected.
func newCmdConfigFormat(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "format [value]",
		Short: "Show or set the default output format (json|pretty|table|ndjson|csv)",
		Long: "Show the default output format, or set it. json is machine-oriented (the default); " +
			"pretty is human-readable. An explicit --format on any command overrides this, and " +
			"SHOPLAZZA_CLI_FORMAT overrides the stored preference (handy to force json in CI).",
		Example: "  shoplazza config format          # show it\n  shoplazza config format pretty   # set it",
		Args:    cobra.MaximumNArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			path, err := core.DefaultConfigPath()
			if err != nil {
				return output.ErrInternal("resolve config path: %v", err)
			}
			cfg, err := core.LoadConfig(path)
			if err != nil {
				return output.ErrInternal("load config: %v", err)
			}

			if len(args) == 0 {
				current := cfg.Format
				if current == "" {
					current = output.FormatJSON
				}
				return output.PrintAPISuccess(cmd.OutOrStdout(),
					map[string]any{"format": current, "default": cfg.Format == ""}, cmdutil.GetFormat(cmd), "")
			}

			val := args[0]
			if !output.ValidFormat(val) {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"invalid output format "+val, "valid values: json, pretty, table, ndjson, csv")
			}
			cfg.Format = val
			if err := core.SaveConfig(path, cfg); err != nil {
				return output.ErrInternal("save config: %v", err)
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "✓ default output format set to %q\n", val)
			return output.PrintAPISuccess(cmd.OutOrStdout(),
				map[string]any{"format": val}, cmdutil.GetFormat(cmd), "")
		},
	}
}
