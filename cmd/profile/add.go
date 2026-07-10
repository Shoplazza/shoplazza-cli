package profile

import (
	"errors"
	"fmt"
	"net/http"
	"strings"

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
			normalized := normalizeDomain(storeDomain)
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
			if err := validateScopeSubset(scopes, acct.GrantedScopes); err != nil {
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
			meta, _ := internalauth.LoadProfileMeta(internalauth.AuthDir(f.ConfigPath), strings.ToLower(name))
			p.StoreID = meta.StoreID

			err := core.UpdateConfig(f.ConfigPath, core.ConfigLockTimeout, func(c *core.CliConfig) error {
				if c.FindProfile(name) != nil {
					return fmt.Errorf("profile %q already exists", name)
				}
				c.Profiles = append(c.Profiles, p)
				if useFlag || len(c.Profiles) == 1 {
					c.PreviousProfile = c.CurrentProfile
					c.CurrentProfile = name
				}
				return nil
			})
			if err != nil {
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

// validateScopeSubset errors when want ⊄ granted (case-sensitive scope names).
func validateScopeSubset(want, granted []string) error {
	set := make(map[string]struct{}, len(granted))
	for _, s := range granted {
		set[s] = struct{}{}
	}
	for _, s := range want {
		if _, ok := set[s]; !ok {
			return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				fmt.Sprintf("scope %q is not granted to this account", s),
				"re-run 'shoplazza auth login' with the scopes you need (see 'shoplazza auth scopes')")
		}
	}
	return nil
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

// normalizeDomain canonicalizes a user-supplied store domain: trims
// whitespace, strips a leading http(s):// scheme, and drops trailing
// slashes. Local copy of cmd/checkout's normalizeStoreDomain (private there;
// no cross-package import).
func normalizeDomain(s string) string {
	s = strings.TrimSpace(s)
	switch lower := strings.ToLower(s); {
	case strings.HasPrefix(lower, "https://"):
		s = s[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		s = s[len("http://"):]
	}
	return strings.TrimRight(s, "/")
}
