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

// newCmdRename renames a profile, moving its keychain entry and auth
// metadata file, and syncing the config's name/current/previous pointers.
func newCmdRename(f *cmdutil.Factory) *cobra.Command {
	var from, to string
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := core.ValidateProfileName(to); err != nil {
				return output.ErrValidation("%s", err.Error())
			}

			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				p := c.FindProfile(from)
				if p == nil {
					return output.ErrValidation("profile %q not found", from)
				}
				// Case-only renames (e.g. "us" -> "US") find themselves here;
				// only reject a genuinely different existing profile.
				if existing := c.FindProfile(to); existing != nil && existing != p {
					return output.ErrValidation("profile %q already exists (names are case-insensitive)", to)
				}
				oldName := p.Name

				// Move the cached store token, if any.
				oldKey := internalauth.ProfileStoreKey(oldName)
				if tok, gerr := keychain.Get(keychain.ShoplazzaCliService, oldKey); gerr == nil && tok != "" {
					if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.ProfileStoreKey(to), tok); err != nil {
						return output.ErrInternal("failed to move keychain entry: %v", err)
					}
					_ = keychain.Remove(keychain.ShoplazzaCliService, oldKey)
				}

				// Move the auth metadata file, if any.
				authDir := internalauth.AuthDir(f.ConfigPath)
				oldLower := strings.ToLower(oldName)
				if meta, merr := internalauth.LoadProfileMeta(authDir, oldLower); merr == nil && meta.ExpiresAt != "" {
					if err := internalauth.SaveProfileMeta(authDir, strings.ToLower(to), meta); err != nil {
						return output.ErrInternal("failed to move profile metadata: %v", err)
					}
					_ = internalauth.RemoveProfileMeta(authDir, oldLower)
				}

				p.Name = to
				if strings.EqualFold(c.CurrentProfile, oldName) {
					c.CurrentProfile = to
				}
				if strings.EqualFold(c.PreviousProfile, oldName) {
					c.PreviousProfile = to
				}
				return nil
			})
			if err != nil {
				return err
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "profile_rename",
				"from":   from,
				"to":     to,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&from, "from", "", "Existing profile name (required)")
	cmd.Flags().StringVar(&to, "to", "", "New profile name (required)")
	return cmd
}
