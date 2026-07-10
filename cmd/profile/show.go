package profile

import (
	"strings"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdShow(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "show",
		Short: "Show a profile's details (defaults to the current profile)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target := name
			if target == "" {
				target = f.Config.CurrentProfile
			}
			if target == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"no current profile set",
					"pass --name, or run 'shoplazza profile add' / 'shoplazza profile use' to set one")
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

			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"name":        p.Name,
				"account":     p.Account,
				"storeDomain": p.StoreDomain,
				"storeId":     storeID,
				"scopes":      p.Scopes,
				"current":     strings.EqualFold(p.Name, f.Config.CurrentProfile),
				"tokenStatus": internalauth.TokenStatus(meta.ExpiresAt),
				"tokenExpiry": meta.ExpiresAt,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to show (defaults to the current profile)")
	return cmd
}
