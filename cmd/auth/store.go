package auth

import (
	"errors"
	"fmt"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

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
		Long:  "Mint a store token for --store-domain, create or reuse that store's profile, and make it the current context.",
		Example: `  # Switch to a store (finds or creates its profile)
  shoplazza auth store use --store-domain my-store.myshoplazza.com

  # Switch and narrow the profile's scopes
  shoplazza auth store use --store-domain my-store.myshoplazza.com --scope read_product,read_order`,
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			// In a terminal, let a human pick which configured store to switch to
			// instead of erroring; agents/pipes still need --store-domain.
			if storeDomain == "" && cmdutil.Interactive(f) {
				picked, err := pickStoreDomain(f)
				if err != nil {
					return err
				}
				storeDomain = picked
			}
			if storeDomain == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--store-domain is required",
					"pass --store-domain <store>, or run 'shoplazza auth login -s <store>' to add one")
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
					return output.ErrAPIAuthHint(httpErr.StatusCode, httpErr.Body, httpErr.RequestID, hint)
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

			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":           true,
				"action":       "store_use",
				"profile":      name,
				"store_domain": normalized,
				"store_id":     meta.StoreID,
				"scopes":       meta.GrantedScopes,
				"token_status": internalauth.TokenStatus(meta.ExpiresAt),
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Store hostname to switch to (e.g. my-store.myshoplazza.com). Required.")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "Scopes to request for this store's profile (must be a subset of the account's granted scopes); empty keeps/grants the full set")
	return cmd
}

// pickStoreDomain lets a human choose which store to switch to when
// --store-domain is omitted in a terminal: a fuzzy list of the stores that
// already have a profile (the current one marked), or a plain domain input when
// none are configured yet. Non-interactive callers never reach it.
func pickStoreDomain(f *cmdutil.Factory) (string, error) {
	if opts := configuredStoreOptions(f.Config); len(opts) > 0 {
		return interact.SelectFiltered("Which store? (type to filter)", opts)
	}
	return interact.Input("Store domain (e.g. my-store.myshoplaza.com)", func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("store domain is required")
		}
		return nil
	})
}

// configuredStoreOptions builds the store picker's choices from the configured
// profiles: one option per distinct store domain, the current one marked. Pure,
// so it is unit-tested without a terminal.
func configuredStoreOptions(cfg core.CliConfig) []interact.Option {
	current := cfg.CurrentStoreDomain()
	seen := map[string]bool{}
	var opts []interact.Option
	for i := range cfg.Profiles {
		d := cfg.Profiles[i].StoreDomain
		if d == "" || seen[d] {
			continue
		}
		seen[d] = true
		label := d
		if strings.EqualFold(d, current) {
			label = d + " (current)"
		}
		opts = append(opts, interact.Option{Label: label, Value: d})
	}
	return opts
}
