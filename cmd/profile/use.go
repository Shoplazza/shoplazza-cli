package profile

import (
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdUse switches the current profile, either to a named target or back
// to the previously-current one.
func newCmdUse(f *cmdutil.Factory) *cobra.Command {
	var name string
	var previous bool
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Switch the current profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if name == "" && !previous {
				return output.ErrValidation("--name or --previous is required")
			}

			var target string
			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				target = name
				if previous {
					target = c.PreviousProfile
					if target == "" {
						return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
							"no previous profile to switch back to",
							"use 'shoplazza profile use --name <n>'")
					}
				}
				p := c.FindProfile(target)
				if p == nil {
					return output.ErrValidation("profile %q not found", target)
				}
				target = p.Name
				if strings.EqualFold(c.CurrentProfile, p.Name) {
					return nil // already on target
				}
				c.PreviousProfile = c.CurrentProfile
				c.CurrentProfile = p.Name
				return nil
			})
			if err != nil {
				return err
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "profile_use",
				"name":   target,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to switch to")
	cmd.Flags().BoolVar(&previous, "previous", false, "Switch back to the previously-current profile")
	cmd.MarkFlagsMutuallyExclusive("name", "previous")
	return cmd
}
