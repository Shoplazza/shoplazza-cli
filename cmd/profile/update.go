package profile

import (
	"strings"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/keychain"
	"shoplazza-cli-v2/internal/output"

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

				authDir := internalauth.AuthDir(f.ConfigPath)
				_ = keychain.Remove(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(p.Name))
				_ = internalauth.RemoveProfileMeta(authDir, strings.ToLower(p.Name))
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
	cmd.Flags().StringVar(&name, "name", "", "Profile to update (required)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "New scopes to request for this profile (must be a subset of the account's granted scopes)")
	return cmd
}
