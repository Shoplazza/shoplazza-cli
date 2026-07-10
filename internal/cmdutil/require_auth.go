package cmdutil

import (
	"context"
	"errors"
	"os"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"

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
		return output.ErrAPIAuthHint(httpErr.StatusCode, httpErr.Body, hint)
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
func RequireAuth(ctx context.Context, f *Factory, cmd *cobra.Command) error {
	if token := os.Getenv("SHOPLAZZA_ACCESS_TOKEN"); token != "" {
		if u := os.Getenv("SHOPLAZZA_CLI_API_BASE_URL"); u != "" {
			f.Client.SetBaseURL(u)
			f.Client.SetBearerToken(token)
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
	return nil
}
