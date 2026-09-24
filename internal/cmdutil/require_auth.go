package cmdutil

import (
	"context"
	"errors"
	"os"
	"strings"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/spf13/cobra"
)

// AnnotationAuthFree marks a cobra command that performs purely local work
// (no Shoplazza API calls); auth gates skip commands carrying this annotation.
const AnnotationAuthFree = "shoplazza.authfree"

// translateAuthErr maps an AccessTokenReadyForProfile failure to the right
// exit class: a non-2xx exchange response keeps its server message/status
// with a re-auth hint; anything else is a generic auth-class error.
func translateAuthErr(err error) error {
	const hint = "re-authenticate with the scopes this command needs, e.g. " +
		"'shoplazza auth login -s <store> --domain checkout' for checkout extensions " +
		"(run 'shoplazza auth login --help' to list domains)"
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return output.ErrAPIAuthHint(httpErr.StatusCode, httpErr.Body, httpErr.RequestID, hint)
	}
	return output.ErrWithHint(
		output.ExitAuth, output.TypeAuth,
		"store access token unavailable: "+err.Error(),
		hint,
	)
}

// RequireAuth resolves the target profile (4-level: --profile, env, config,
// error) and injects its store base URL + bearer token onto f.Client. Returns
// a typed ExitError on any failure.
//
// SHOPLAZZA_ACCESS_TOKEN bypasses login/minting (CI / test injection), but a
// store target is still required: SHOPLAZZA_CLI_API_BASE_URL wins outright
// (even over a configured profile); otherwise the profile resolves the store
// domain. Neither available is a loud error, not a silent no-op.
//
// It also injects the cli-user-id header (audit attribution) when one is
// resolvable — see CliUserID.
func RequireAuth(ctx context.Context, f *Factory, cmd *cobra.Command) error {
	if token := os.Getenv("SHOPLAZZA_ACCESS_TOKEN"); token != "" {
		// An injected token skips login state, so the audit id must be injected
		// too: a stale local login would attribute the call to the wrong user.
		if u := os.Getenv("SHOPLAZZA_CLI_API_BASE_URL"); u != "" {
			f.Client.SetBaseURL(u)
			f.Client.SetBearerToken(token)
			f.Client.SetCliUserID(CliUserIDEnv())
			return nil
		}
		p, err := ResolveProfile(f, cmd)
		if err != nil {
			return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
				"SHOPLAZZA_ACCESS_TOKEN is set but no store target is available",
				"set SHOPLAZZA_CLI_API_BASE_URL, or provide a profile (config.json / --profile / SHOPLAZZA_CLI_PROFILE)")
		}
		f.Client.SetBaseURL("https://" + p.StoreDomain)
		f.Client.SetBearerToken(token)
		f.Client.SetCliUserID(CliUserIDEnv())
		return nil
	}

	p, err := ResolveProfile(f, cmd)
	if err != nil {
		return err
	}
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := mgr.AccessTokenReadyForProfile(ctx, f.ConfigPath, *p)
	if err != nil {
		return translateAuthErr(err)
	}
	f.Client.SetBaseURL("https://" + p.StoreDomain)
	f.Client.SetBearerToken(tok)
	f.Client.SetCliUserID(cliUserIDFrom(mgr))
	return nil
}

// EnvCliUserID overrides the cli-user-id header value.
const EnvCliUserID = "SHOPLAZZA_CLI_USER_ID"

// CliUserIDEnv returns the env override, or "" when unset.
func CliUserIDEnv() string { return strings.TrimSpace(os.Getenv(EnvCliUserID)) }

// cliUserIDFrom resolves the cli-user-id header value (audit attribution):
// the env override first, else the login user id captured at login time.
// Best-effort — an unresolvable id omits the header rather than failing the
// command, and no network call is made on this path.
func cliUserIDFrom(mgr *internalauth.Manager) string {
	if v := CliUserIDEnv(); v != "" {
		return v
	}
	state, err := mgr.LoadState()
	if err != nil {
		return ""
	}
	return state.UserID
}

// CliUserID resolves the header value for callers that build their own store
// client instead of going through the gate.
func CliUserID(f *Factory) string {
	return cliUserIDFrom(internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient))
}
