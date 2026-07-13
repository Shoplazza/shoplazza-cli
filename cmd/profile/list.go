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
			acct := f.Config.Account()
			items := make([]any, 0, len(f.Config.Profiles))
			for _, p := range f.Config.Profiles {
				meta, _ := internalauth.LoadProfileMeta(authDir, strings.ToLower(p.Name))
				storeID := p.StoreID
				if storeID == "" {
					storeID = meta.StoreID
				}
				items = append(items, map[string]any{
					"name":        p.Name,
					"account":     p.Account,
					"storeDomain": p.StoreDomain,
					"storeId":     storeID,
					"scopes":      effectiveScopes(p, meta, acct),
					"current":     strings.EqualFold(p.Name, f.Config.CurrentProfile),
				})
			}
			return output.PrintBody(cmd.OutOrStdout(), items, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	return cmd
}

// effectiveScopes resolves the scope set a profile's store token carries (or
// will request on its next mint): the minted grant if present, else the
// explicit per-profile narrowing, else the account's full granted set (the
// nil-means-inherit default). Shared by 'profile list' and 'profile info' so
// both report the profile's real scopes instead of a bare null.
func effectiveScopes(p core.ProfileConfig, meta internalauth.ProfileMeta, acct *core.AccountConfig) []string {
	if len(meta.GrantedScopes) > 0 {
		return meta.GrantedScopes
	}
	if len(p.Scopes) > 0 {
		return p.Scopes
	}
	if acct != nil && strings.EqualFold(acct.Name, p.Account) {
		return acct.GrantedScopes
	}
	return nil
}
