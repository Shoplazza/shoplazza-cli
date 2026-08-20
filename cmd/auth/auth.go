package auth

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

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
		mergeScopes     bool
	)

	cmd := &cobra.Command{
		Use:   "login",
		Short: "Log in to your Shoplazza account",
		Args:  cobra.NoArgs,
		// Interactive: waits on the browser OAuth callback.
		Annotations: map[string]string{cmdutil.AnnotationNotScannable: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			if len(scope) > 0 {
				if err := internalauth.ValidateScopes(scope); err != nil {
					return output.ErrWithHint(
						output.ExitValidation, output.TypeValidation, err.Error(),
						"Run 'shoplazza auth scopes' to see all supported scopes")
				}
			}
			expandDomains := func(domains []string) ([]string, error) {
				scopes, err := internalauth.ExpandDomains(domains)
				if err != nil {
					return nil, output.ErrWithHint(
						output.ExitValidation, output.TypeValidation, err.Error(),
						"Pass a top-level CLI command name as --domain, e.g. products, orders, shop")
				}
				return scopes, nil
			}
			domainScopes, err := expandDomains(domain)
			if err != nil {
				return err
			}

			// Interactive path: ask only for what the flags left unanswered, then
			// re-expand. Invalid values already failed above, before any prompt.
			// plan() returns no steps for pipes, CI and agents, so everything
			// below stays exactly as it was.
			wizardRan := false
			if steps := plan(loginFlags{
				storeDomain: storeDomain,
				domain:      domain,
				scope:       scope,
				uat:         firstNonEmpty(uat, os.Getenv("SHOPLAZZA_UAT")),
				mergeScopes: mergeScopes,
			}, cmdutil.Interactive(f)); len(steps) > 0 {
				if err := runLoginWizard(steps, &storeDomain, &domain); err != nil {
					return err
				}
				if domainScopes, err = expandDomains(domain); err != nil {
					return err
				}
				wizardRan = true
			}

			// scope is OPTIONAL: pure-account login (no flags) is valid.
			effectiveScopes := internalauth.DedupePreserveOrder(append(append([]string{}, scope...), domainScopes...))

			// Authorization REPLACES the account's granted set server-side, and
			// SyncAfterLogin then trims every profile to it — so a narrow re-login
			// (e.g. just read_inventory) revokes unrelated scopes mid-task,
			// including other profiles'. --merge-scopes re-requests the union;
			// the default keeps the historical replace semantics untouched.
			keptFromGrant := 0
			if mergeScopes && len(effectiveScopes) > 0 {
				if acct := f.Config.Account(); acct != nil {
					effectiveScopes, keptFromGrant = unionWithGranted(effectiveScopes, acct.GrantedScopes)
				}
			}

			normalizedStore := ""
			if storeDomain != "" {
				normalizedStore = cmdutil.NormalizeStoreDomain(storeDomain)
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

			// After the wizard the summary is the card, which expands the "all"
			// sentinel into the domains it granted. Untouched otherwise.
			if wizardRan {
				interact.Summary(f.IOStreams.ErrOut, loginSummaryRows(normalizedStore, domain, scope)...)
			} else {
				fmt.Fprintf(f.IOStreams.ErrOut, "Summary:\n")
				if normalizedStore != "" {
					fmt.Fprintf(f.IOStreams.ErrOut, "  Store:      %s\n", normalizedStore)
				} else {
					fmt.Fprintf(f.IOStreams.ErrOut, "  Store:      (account only)\n")
				}
				fmt.Fprintf(f.IOStreams.ErrOut, "  Scopes (%d): %s\n", len(effectiveScopes), strings.Join(effectiveScopes, ", "))
			}
			if keptFromGrant > 0 {
				fmt.Fprintf(f.IOStreams.ErrOut, "  (--merge-scopes kept %d previously granted scope(s))\n", keptFromGrant)
			}
			fmt.Fprintf(f.IOStreams.ErrOut, "\n")

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
			// GrantedScopes is only populated by a store-token exchange, so
			// only validate --scope when a store exchange actually happened; an
			// account-only login leaves it empty.
			if storeArg != "" {
				if err := cmdutil.ValidateScopeSubset(scope, result.Status.GrantedScopes); err != nil {
					return err
				}
			}
			// Default path passes the raw --scope flag exactly as before. Under
			// --merge-scopes the profile records the full effective set instead,
			// so a later lazy re-mint keeps the merged token's reach.
			syncScopes := scope
			if mergeScopes && len(effectiveScopes) > 0 {
				syncScopes = effectiveScopes
			}
			profileName, err := SyncAfterLogin(f, result, storeArg, syncScopes, f.IOStreams.ErrOut)
			if err != nil {
				return output.ErrInternal("failed to sync profile state: %v", err)
			}
			// Persist the login-time exchange under the profile key so the new
			// profile lands ready ("valid"), instead of re-minting on first use.
			// Best-effort: a failed write self-heals via the Gate's lazy mint.
			if profileName != "" && result.StoreToken != nil {
				if perr := internalauth.PersistProfileToken(internalauth.AuthDir(f.ConfigPath), profileName, result.StoreToken); perr != nil {
					fmt.Fprintf(f.IOStreams.ErrOut, "warning: store token not cached (will re-mint on next use): %v\n", perr)
				}
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
	cmd.Flags().BoolVar(&mergeScopes, "merge-scopes", false, "Also re-request every previously granted scope (authorization replaces the grant server-side, so a narrow re-login otherwise revokes scopes parallel tasks rely on).")
	return cmd
}

// firstNonEmpty returns the first non-empty string, or "".
func firstNonEmpty(vals ...string) string {
	for _, v := range vals {
		if v != "" {
			return v
		}
	}
	return ""
}

// unionWithGranted merges previously granted account scopes into a scope
// request, preserving the request's order first. Returns the merged set and
// how many scopes were carried over from the prior grant.
func unionWithGranted(requested, granted []string) ([]string, int) {
	merged := internalauth.DedupePreserveOrder(append(append([]string{}, requested...), granted...))
	return merged, len(merged) - len(internalauth.DedupePreserveOrder(requested))
}

// domainFlagHelp builds the --domain help text from the live scope map.
func domainFlagHelp() string {
	return "Requested CLI domains (top-level command names, comma-separated). " +
		"e.g. --domain products,orders. Each domain expands into the OAuth scopes " +
		"that module needs.\nAvailable: " +
		strings.Join(internalauth.TopLevelDomains(), ", ") + ", " + internalauth.DomainAll + "."
}

func newCmdLogout(f *cmdutil.Factory) *cobra.Command {
	return &cobra.Command{
		Use:   "logout",
		Short: "Log out from the current store",
		// Mutates the local keychain.
		Annotations: map[string]string{cmdutil.AnnotationNotScannable: "true"},
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
		Use:         "status",
		Short:       "Show current authentication status",
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			manager := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
			status, err := manager.CurrentStatus()
			if err != nil {
				return output.Errorf(output.ExitInternal, output.TypeInternal, "failed to read auth state: %s", err.Error())
			}

			out := map[string]any{
				"logged_in":      status.LoggedIn,
				"account":        status.Account,
				"user_id":        status.UserID,
				"granted_scopes": status.GrantedScopes,
				"uat_available":  status.UATAvailable,
				"uat_expires_at": status.UATExpiresAt,
				"profiles":       internalauth.ProfileRows(f.Config, internalauth.AuthDir(f.ConfigPath)),
			}
			if len(status.Stores) > 0 {
				out["stores"] = status.Stores
			}

			return output.PrintBody(cmd.OutOrStdout(), out, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
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
				"current_store":    manager.Config.CurrentStoreDomain(),
				"granted_scopes":   state.GrantedScopes,
				"supported_scopes": internalauth.SupportedScopes(),
			})
		},
	}
}
