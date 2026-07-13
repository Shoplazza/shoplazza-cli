package profile

import (
	"strings"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdInfo(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show a profile's details (defaults to the current profile)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := currentOrNamed(f, name)
			if err != nil {
				return err
			}
			p := f.Config.FindProfile(target)
			if p == nil {
				return output.ErrValidation("profile %q not found", target)
			}

			authDir := internalauth.AuthDir(f.ConfigPath)
			meta, _ := internalauth.LoadProfileMeta(authDir, strings.ToLower(p.Name))
			storeID := p.StoreID
			if storeID == "" {
				storeID = meta.StoreID
			}

			// scopes reflects the store token's effective scope set (the minted
			// grant, else the requested narrowing, else the account's full set),
			// so it stays consistent with the token instead of a bare null.
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"name":         p.Name,
				"account":      p.Account,
				"store_domain": p.StoreDomain,
				"store_id":     storeID,
				"scopes":       internalauth.EffectiveScopes(*p, meta, f.Config.Account()),
				"current":      strings.EqualFold(p.Name, f.Config.CurrentProfile),
				"token_status": internalauth.TokenStatus(meta.ExpiresAt),
				"token_expiry": meta.ExpiresAt,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to show (defaults to the current profile)")
	return cmd
}
