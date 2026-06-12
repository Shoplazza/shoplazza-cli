package cmdutil

import (
	"context"
	"errors"
	"os"

	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
)

// AnnotationAuthFree marks a cobra command that performs purely local work
// (no Shoplazza API calls); auth gates skip commands carrying this annotation.
const AnnotationAuthFree = "shoplazza.authfree"

// RequireAuth verifies an account UAT exists, resolves the current store,
// mints/refreshes that store's token, and writes it onto f.Client as the
// Access-Token bearer. Returns a typed ExitError on any failure.
//
// SHOPLAZZA_ACCESS_TOKEN bypasses the gate entirely (CI / test injection); its
// value is wired onto f.Client by NewDefaultFactory.
func RequireAuth(ctx context.Context, f *Factory) error {
	if os.Getenv("SHOPLAZZA_ACCESS_TOKEN") != "" {
		return nil
	}
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	status, err := mgr.CurrentStatus()
	if err != nil {
		return output.ErrAuth("failed to check login status: %v", err)
	}
	if !status.LoggedIn {
		return output.ErrWithHint(
			output.ExitAuth, output.TypeAuth,
			"not logged in",
			"run 'shoplazza auth login' to authenticate",
		)
	}
	storeDomain := f.Config.StoreDomain
	if storeDomain == "" {
		return output.ErrWithHint(
			output.ExitValidation, output.TypeValidation,
			"no current store selected",
			"run 'shoplazza auth store use --store-domain <domain>' to select a store",
		)
	}
	tok, err := mgr.AccessTokenReady(ctx, storeDomain)
	if err != nil {
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
	if f.Client != nil {
		f.Client.SetBearerToken(tok)
	}
	return nil
}
