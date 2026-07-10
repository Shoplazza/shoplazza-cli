package auth

import (
	"errors"
	"fmt"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdStore(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "store",
		Short: "Manage the current store token context",
	}
	cmd.AddCommand(newCmdStoreUse(f))
	return cmd
}

func newCmdStoreUse(f *cmdutil.Factory) *cobra.Command {
	var (
		storeDomain string
		scope       []string
	)
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Request a store token and set it as the current store",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if storeDomain == "" {
				return output.ErrValidation("--store-domain is required")
			}
			_, normalized := parseStoreDomain(storeDomain)
			if normalized == "" {
				return output.ErrValidation("--store-domain must not be empty")
			}
			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			status, err := manager.CurrentStatus()
			if err != nil {
				return output.Errorf(output.ExitInternal, output.TypeInternal, "failed to read auth state: %s", err.Error())
			}
			if !status.LoggedIn {
				return output.ErrWithHint(output.ExitAuth, output.TypeAuth,
					"not logged in",
					"Run 'shoplazza auth login' to authenticate first")
			}
			newStatus, err := manager.UseStore(cmd.Context(), normalized)
			if err != nil {
				var httpErr *client.HTTPError
				if errors.As(err, &httpErr) {
					// 5xx stays a masked server error; client-side failures on the
					// store-token exchange are auth-class (scope/permission/wrong store).
					if httpErr.StatusCode >= 500 {
						return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "")
					}
					// Omit the "grant scopes" hint on 404: a wrong store domain
					// can't be fixed by re-authorizing.
					hint := ""
					if httpErr.StatusCode != 404 {
						hint = fmt.Sprintf(
							"to grant store scopes, run 'shoplazza auth login -s %s --scope <scope>' (or --domain). Run 'shoplazza auth scopes' to list scopes.",
							normalized)
					}
					return output.ErrAPIAuthHint(httpErr.StatusCode, httpErr.Body, hint)
				}
				return output.Errorf(output.ExitAuth, output.TypeAuth, "failed to obtain store token: %s", err.Error())
			}
			// Validate --scope against THIS store's fresh grant (UseStore always
			// exchanges, so newStatus.GrantedScopes is ground-truth, unlike the
			// account-level GrantedScopes which SyncAfterLogin overwrites per-store
			// and which is empty after an account-only login).
			if err := cmdutil.ValidateScopeSubset(scope, newStatus.GrantedScopes); err != nil {
				return err
			}
			if err := SyncAfterLogin(f, internalauth.LoginResult{Status: newStatus}, normalized, scope, f.IOStreams.ErrOut); err != nil {
				return output.ErrInternal("failed to sync profile state: %v", err)
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "store_use",
				"status": newStatus,
			})
		},
	}
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Store hostname to switch to (e.g. my-store.myshoplazza.com). Required.")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "Scopes to request for this store's profile (must be a subset of the account's granted scopes); empty keeps/grants the full set")
	return cmd
}
