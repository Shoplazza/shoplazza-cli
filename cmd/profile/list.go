package profile

import (
	"strings"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			authDir := internalauth.AuthDir(f.ConfigPath)
			items := make([]any, 0, len(f.Config.Profiles))
			for _, p := range f.Config.Profiles {
				items = append(items, map[string]any{
					"name":        p.Name,
					"account":     p.Account,
					"storeDomain": p.StoreDomain,
					"storeId":     resolveStoreID(authDir, p),
					"scopes":      p.Scopes,
					"current":     strings.EqualFold(p.Name, f.Config.CurrentProfile),
				})
			}
			return output.PrintBody(cmd.OutOrStdout(), items, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	return cmd
}

// resolveStoreID prefers the persisted config value (backfilled at `add`
// time); it falls back to the profile's metadata file for profiles that
// predate that backfill.
func resolveStoreID(authDir string, p core.ProfileConfig) string {
	if p.StoreID != "" {
		return p.StoreID
	}
	meta, _ := internalauth.LoadProfileMeta(authDir, strings.ToLower(p.Name))
	return meta.StoreID
}
