package auth

import (
	"errors"
	"fmt"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
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

// newCmdStoreUse mints a store token under the profile model (find-or-create
// the store's profile, exchange eagerly, set it current). One store, one
// profile, one token — the legacy account-level store slot is no longer
// written.
func newCmdStoreUse(f *cmdutil.Factory) *cobra.Command {
	var (
		storeDomain string
		scope       []string
	)
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Request a store token and set its profile as current",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if storeDomain == "" {
				return output.ErrValidation("--store-domain is required")
			}
			normalized := cmdutil.NormalizeStoreDomain(storeDomain)
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
			email := status.Account
			if email == "" {
				if acct := f.Config.Account(); acct != nil {
					email = acct.Name
				}
			}

			// Fresh read for name resolution: f.Config is a process-start
			// snapshot and misses a login from this same process.
			cfg, err := core.LoadConfig(f.ConfigPath)
			if err != nil {
				return output.ErrInternal("failed to load config: %v", err)
			}
			name := ""
			isNew := false
			if existing := cfg.FindProfileByStore(normalized); existing != nil {
				name = existing.Name
			} else {
				name = core.DeriveProfileName(normalized, func(n string) bool { return cfg.FindProfile(n) != nil })
				isNew = true
			}
			p := core.ProfileConfig{Name: name, Account: email, StoreDomain: normalized, Scopes: scope}

			// Mint before touching config: a bad domain or scope failure must
			// not leave a profile behind.
			authDir := internalauth.AuthDir(f.ConfigPath)
			if _, err := manager.ExchangeForProfile(cmd.Context(), authDir, p); err != nil {
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

			// Validate --scope against THIS store's fresh grant (the exchange
			// always runs, so meta.GrantedScopes is ground-truth, unlike the
			// account-level grant which is empty after an account-only login).
			meta, _ := internalauth.LoadProfileMeta(authDir, name)
			if err := cmdutil.ValidateScopeSubset(scope, meta.GrantedScopes); err != nil {
				// Leave no freshly-minted residue behind a failed store use.
				internalauth.ForgetProfileToken(authDir, name)
				return err
			}
			p.StoreID = meta.StoreID

			err = core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				if existing := c.FindProfileByStore(normalized); existing != nil {
					if scope != nil {
						existing.Scopes = scope
					}
					if existing.StoreID == "" {
						existing.StoreID = meta.StoreID
					}
					c.PreviousProfile, c.CurrentProfile = c.CurrentProfile, existing.Name
					return nil
				}
				c.Profiles = append(c.Profiles, p)
				c.PreviousProfile, c.CurrentProfile = c.CurrentProfile, p.Name
				return nil
			})
			if err != nil {
				if isNew {
					internalauth.ForgetProfileToken(authDir, name)
				}
				return output.ErrInternal("failed to save profile: %v", err)
			}

			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"ok":           true,
				"action":       "store_use",
				"profile":      name,
				"store_domain": normalized,
				"store_id":     meta.StoreID,
				"scopes":       meta.GrantedScopes,
				"token_status": internalauth.TokenStatus(meta.ExpiresAt),
			})
		},
	}
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Store hostname to switch to (e.g. my-store.myshoplazza.com). Required.")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "Scopes to request for this store's profile (must be a subset of the account's granted scopes); empty keeps/grants the full set")
	return cmd
}
