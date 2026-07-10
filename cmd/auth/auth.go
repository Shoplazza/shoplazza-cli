package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// NewCmdAuth creates the auth command group.
func NewCmdAuth(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "auth",
		Short: "Authentication commands",
	}

	cmd.AddCommand(
		newCmdLogin(f),
		newCmdLogout(f),
		newCmdStatus(f),
		newCmdScopes(f),
		newCmdStore(f),
	)

	return cmd
}

func newCmdLogin(f *cmdutil.Factory) *cobra.Command {
	var (
		storeDomain     string
		scope           []string
		domain          []string
		uat             string
		timeoutSec      int
		pollIntervalSec int
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to your Shoplazza account",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(scope) > 0 {
				if err := internalauth.ValidateScopes(scope); err != nil {
					return output.ErrWithHint(
						output.ExitValidation, output.TypeValidation, err.Error(),
						"Run 'shoplazza auth scopes' to see all supported scopes")
				}
			}
			domainScopes, err := expandLoginDomains(domain)
			if err != nil {
				return output.ErrWithHint(
					output.ExitValidation, output.TypeValidation, err.Error(),
					"Pass a top-level CLI command name as --domain, e.g. products, orders, shop")
			}
			// scope is OPTIONAL: pure-account login (no flags) is valid.
			effectiveScopes := internalauth.DedupePreserveOrder(append(append([]string{}, scope...), domainScopes...))

			normalizedStore := ""
			if storeDomain != "" {
				_, normalizedStore = parseStoreDomain(storeDomain)
				if normalizedStore == "" {
					return output.ErrValidation("--store-domain must not be empty")
				}
			}

			// Interactive store login requires scopes; the --uat / SHOPLAZZA_UAT path
			// is exempt (the store token inherits the UAT's account scopes).
			if normalizedStore != "" && len(effectiveScopes) == 0 && uat == "" && os.Getenv("SHOPLAZZA_UAT") == "" {
				return output.ErrWithHint(
					output.ExitValidation, output.TypeValidation,
					"selecting a store with --store-domain requires at least one scope",
					"pass --scope or --domain, e.g. --domain products,orders. Run 'shoplazza auth scopes' to list scopes.")
			}

			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)

			fmt.Fprintf(f.IOStreams.ErrOut, "Summary:\n")
			if normalizedStore != "" {
				fmt.Fprintf(f.IOStreams.ErrOut, "  Store:      %s\n", normalizedStore)
			} else {
				fmt.Fprintf(f.IOStreams.ErrOut, "  Store:      (account only)\n")
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "  Scopes (%d): %s\n\n", len(effectiveScopes), strings.Join(effectiveScopes, ", "))

			result, err := manager.Login(
				context.Background(),
				normalizedStore,
				effectiveScopes,
				uat,
				time.Duration(timeoutSec)*time.Second,
				time.Duration(pollIntervalSec)*time.Second,
				func(authorizeURL string) {
					fmt.Fprintf(f.IOStreams.ErrOut, "Open this URL to authorize in your browser:\n\n  %s\n\n", authorizeURL)
					fmt.Fprintf(f.IOStreams.ErrOut, "Waiting for authorization...\n")
				},
			)
			if err != nil {
				return output.ErrWithHint(
					output.ExitAuth, output.TypeAuth,
					fmt.Sprintf("login failed: %s", err.Error()),
					"Run 'shoplazza auth login' to retry")
			}

			fmt.Fprintf(f.IOStreams.ErrOut, "\nOK: Login successful!\n")
			if result.StoreWarning != "" {
				fmt.Fprintf(f.IOStreams.ErrOut, "  warning: %s\n", result.StoreWarning)
			}
			if result.Status.CurrentStore != "" {
				fmt.Fprintf(f.IOStreams.ErrOut, "  Current store: %s\n", result.Status.CurrentStore)
			}
			if len(result.Status.GrantedScopes) > 0 {
				fmt.Fprintf(f.IOStreams.ErrOut, "  Granted scopes: %s\n", strings.Join(result.Status.GrantedScopes, " "))
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "  UAT: %s\n", result.UAT)

			// If the requested --store-domain failed live validation, don't create
			// or activate a profile for it (result.Status.CurrentStore is already "").
			storeArg := normalizedStore
			if result.StoreWarning != "" {
				storeArg = ""
			}
			// GrantedScopes is only populated by a store-token exchange; an
			// account-only login never touches it, so only validate when a store
			// exchange actually happened.
			if storeArg != "" {
				if err := cmdutil.ValidateScopeSubset(scope, result.Status.GrantedScopes); err != nil {
					return err
				}
			}
			if err := SyncAfterLogin(f, result, storeArg, scope, f.IOStreams.ErrOut); err != nil {
				return output.ErrInternal("failed to sync profile state: %v", err)
			}

			// Store warning is shown in the stderr summary only, not echoed in the JSON.
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "login",
				"flow":   result.Flow,
				"uat":    result.UAT,
				"status": result.Status,
			})
		},
	}

	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Optional store hostname to select on login (e.g. my-store.myshoplazza.com). When set on an interactive login, also pass --scope or --domain. Distinct from --domain.")
	cmd.Flags().StringSliceVar(&scope, "scope", nil, "Requested OAuth scopes (space or comma separated). e.g. read_product,write_product")
	cmd.Flags().StringSliceVar(&domain, "domain", nil, domainFlagHelp())
	cmd.Flags().StringVar(&uat, "uat", "", "Log in non-interactively with an existing account UAT (skips the browser; obtain it from 'auth login' on another machine).")
	cmd.Flags().IntVar(&timeoutSec, "timeout", 300, "Web-flow polling timeout in seconds.")
	cmd.Flags().IntVar(&pollIntervalSec, "poll-interval", 2, "Web-flow poll interval in seconds.")
	return cmd
}

// domainFlagHelp builds the --domain help text from the live scope map.
func domainFlagHelp() string {
	return "Requested CLI domains (top-level command names, comma-separated). " +
		"e.g. --domain products,orders. Each domain expands into the OAuth scopes " +
		"that module needs.\nAvailable: " +
		strings.Join(internalauth.TopLevelDomains(), ", ") + ", " + internalauth.DomainAll + "."
}

// expandLoginDomains expands --domain values into OAuth scopes. Beyond the
// API-module domains handled by internalauth.ExpandDomains, it accepts the
// alias "app": the scopes app-extension development needs. themes, checkout,
// and theme-extension uploads all authorize via the themes scope, so
// `auth login -s <store> --domain app` grants read_themes + write_themes.
func expandLoginDomains(domains []string) ([]string, error) {
	rest := make([]string, 0, len(domains))
	var appScopes []string
	for _, d := range domains {
		if d == "app" {
			// themes, checkout, and theme-extension uploads all authorize via the
			// themes scope, so that single domain covers app-extension development.
			s, err := internalauth.ExpandDomain("themes")
			if err != nil {
				return nil, err
			}
			appScopes = append(appScopes, s...)
			continue
		}
		rest = append(rest, d)
	}
	scopes, err := internalauth.ExpandDomains(rest)
	if err != nil {
		return nil, err
	}
	return internalauth.DedupePreserveOrder(append(scopes, appScopes...)), nil
}

func newCmdLogout(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out from the current store",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			_, err := manager.Logout()
			if err != nil {
				return output.Errorf(output.ExitAPI, output.TypeAuth, "logout failed: %s", err.Error())
			}
			if err := wipeV2OnLogout(f); err != nil {
				return output.ErrInternal("failed to clear profile state: %v", err)
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"ok":     true,
				"action": "logout",
			})
		},
	}
}

func newCmdStatus(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "status",
		Short: "Show current authentication status",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			status, err := manager.CurrentStatus()
			if err != nil {
				return output.Errorf(output.ExitInternal, output.TypeInternal, "failed to read auth state: %s", err.Error())
			}
			return output.PrintJSON(cmd.OutOrStdout(), status)
		},
	}
}

// parseStoreDomain splits "https://store.myshoplazza.com/" into ("https",
// "store.myshoplazza.com"). Missing scheme defaults to https.
func parseStoreDomain(raw string) (scheme, host string) {
	d := strings.TrimSpace(raw)
	switch {
	case strings.HasPrefix(d, "https://"):
		return "https", strings.TrimRight(strings.TrimPrefix(d, "https://"), "/")
	case strings.HasPrefix(d, "http://"):
		return "http", strings.TrimRight(strings.TrimPrefix(d, "http://"), "/")
	default:
		return "https", strings.TrimRight(d, "/")
	}
}

func newCmdScopes(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "scopes",
		Short: "Show supported scopes and the account-level scopes currently granted",
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			state, err := manager.LoadState()
			if err != nil {
				return output.Errorf(output.ExitInternal, output.TypeInternal, "failed to read auth state: %s", err.Error())
			}
			return output.PrintJSON(cmd.OutOrStdout(), map[string]any{
				"current_store":    manager.Config.StoreDomain,
				"granted_scopes":   state.GrantedScopes,
				"supported_scopes": internalauth.SupportedScopes(),
			})
		},
	}
}
