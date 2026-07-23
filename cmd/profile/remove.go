package profile

import (
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdRemove drops a profile: its cached store token, its auth metadata
// file, and its config record. Account-level credentials (UAT/partner
// token) are untouched. If the removed profile was current/previous, those
// pointers are repaired.
func newCmdRemove(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				p := c.FindProfile(name)
				if p == nil {
					return output.ErrValidation("profile %q not found", name)
				}
				removed := p.Name

				internalauth.ForgetProfileToken(internalauth.AuthDir(f.ConfigPath), removed)

				idx := -1
				for i := range c.Profiles {
					if strings.EqualFold(c.Profiles[i].Name, removed) {
						idx = i
						break
					}
				}
				c.Profiles = append(c.Profiles[:idx], c.Profiles[idx+1:]...)

				if strings.EqualFold(c.CurrentProfile, removed) {
					if len(c.Profiles) > 0 {
						c.CurrentProfile = c.Profiles[0].Name
					} else {
						c.CurrentProfile = ""
					}
				}
				if strings.EqualFold(c.PreviousProfile, removed) {
					c.PreviousProfile = ""
				}
				return nil
			})
			if err != nil {
				return err
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "profile_remove",
				"name":   name,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to remove (required)")
	return cmd
}
