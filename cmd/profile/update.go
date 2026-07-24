package profile

import (
	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdUpdate changes a profile's scope subset. The cached store access
// token is cleared so the next command re-exchanges under the new scopes.
func newCmdUpdate(f *cmdutil.Factory) *cobra.Command {
	var (
		name   string
		scopes []string
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a profile's scopes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			name, nerr := currentOrNamed(f, name)
			if nerr != nil {
				return nerr
			}
			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				p := c.FindProfile(name)
				if p == nil {
					return output.ErrValidation("profile %q not found", name)
				}
				acct := c.Account()
				var granted []string
				if acct != nil {
					granted = acct.GrantedScopes
				}
				if err := cmdutil.ValidateScopeSubset(scopes, granted); err != nil {
					return err
				}
				p.Scopes = scopes

				internalauth.ForgetProfileToken(internalauth.AuthDir(f.ConfigPath), p.Name)
				return nil
			})
			if err != nil {
				return err
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "profile_update",
				"name":   name,
				"scopes": scopes,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to update (defaults to the current profile)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "New scopes to request for this profile (must be a subset of the account's granted scopes)")
	return cmd
}
