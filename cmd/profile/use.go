package profile

import (
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

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

			// Pre-lock check: if we're already on the resolved target, skip
			// the locked read-modify-write entirely — nothing to persist.
			if cfg, lerr := core.LoadConfig(f.ConfigPath); lerr == nil {
				want := name
				if previous {
					want = cfg.PreviousProfile
				}
				if p := cfg.FindProfile(want); p != nil && strings.EqualFold(cfg.CurrentProfile, p.Name) {
					return output.PrintBody(cmd.OutOrStdout(), map[string]any{
						"ok":     true,
						"action": "profile_use",
						"name":   p.Name,
					}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
				}
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
	_ = cmd.RegisterFlagCompletionFunc("name", cmdutil.ProfileNameCompletionFunc(f))
	return cmd
}
