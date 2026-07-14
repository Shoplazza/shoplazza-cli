package profile

import (
	"errors"
	"fmt"
	"net/http"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdAdd(f *cmdutil.Factory) *cobra.Command {
	var (
		name        string
		storeDomain string
		scopes      []string
		useFlag     bool
	)

	cmd := &cobra.Command{
		Use:   "add",
		Short: "Add a new profile (mints and persists its store access token)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			if err := core.ValidateProfileName(name); err != nil {
				return output.ErrValidation("%s", err.Error())
			}
			normalized := cmdutil.NormalizeStoreDomain(storeDomain)
			if normalized == "" {
				return output.ErrValidation("--store-domain is required")
			}

			acct := f.Config.Account()
			if acct == nil {
				return output.ErrWithHint(output.ExitAuth, output.TypeAuth,
					"not logged in", "run 'shoplazza auth login' first")
			}
			if f.Config.FindProfile(name) != nil {
				return output.ErrValidation("profile %q already exists (names are case-insensitive)", name)
			}
			if err := cmdutil.ValidateScopeSubset(scopes, acct.GrantedScopes); err != nil {
				return err
			}

			p := core.ProfileConfig{
				Name:        name,
				Account:     acct.Name,
				StoreDomain: normalized,
				Scopes:      scopes,
			}
			mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			if _, err := mgr.ExchangeForProfile(ctx, internalauth.AuthDir(f.ConfigPath), p); err != nil {
				return translateExchangeErr(err)
			}
			// Backfill the numeric store id the exchange resolved.
			meta, _ := internalauth.LoadProfileMeta(internalauth.AuthDir(f.ConfigPath), name)
			p.StoreID = meta.StoreID

			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				if c.FindProfile(name) != nil {
					return output.ErrValidation("profile %q already exists (names are case-insensitive)", name)
				}
				c.Profiles = append(c.Profiles, p)
				if useFlag || len(c.Profiles) == 1 {
					c.PreviousProfile = c.CurrentProfile
					c.CurrentProfile = name
				}
				return nil
			})
			if err != nil {
				var exitErr *output.ExitError
				if errors.As(err, &exitErr) {
					return exitErr
				}
				return output.ErrInternal("failed to save profile: %v", err)
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok":      true,
				"action":  "profile_add",
				"name":    p.Name,
				"account": p.Account,
			}, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}

	cmd.Flags().StringVar(&name, "name", "", "Profile name (required)")
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Store hostname to bind this profile to, e.g. my-store.myshoplazza.com (required)")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "Scopes to request for this profile (must be a subset of the account's granted scopes); empty grants the account's full scope set")
	cmd.Flags().BoolVar(&useFlag, "use", false, "Set this profile as current after adding it")
	return cmd
}

// translateExchangeErr classifies an ExchangeForProfile failure: 404 means the
// store domain doesn't exist (a fixable validation error); 5xx stays a masked
// server error; anything else in between is an auth-class failure (bad UAT,
// missing scope) with a re-login hint.
func translateExchangeErr(err error) error {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		if httpErr.StatusCode == http.StatusNotFound {
			return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				fmt.Sprintf("store not found: %s", httpErr.Body),
				"check --store-domain and try again")
		}
		if httpErr.StatusCode >= 500 {
			return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "")
		}
		return output.ErrAPIAuthHint(httpErr.StatusCode, httpErr.Body,
			"re-run 'shoplazza auth login' with the scopes you need (see 'shoplazza auth scopes')")
	}
	return output.ErrWithHint(output.ExitAuth, output.TypeAuth,
		"exchange failed: "+err.Error(),
		"run 'shoplazza auth login' first")
}
