package theme_extension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"shoplazza-cli-v2/internal/app"
	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

// envAccessToken is the CI/test injection bypass honored across the CLI (see
// cmdutil.RequireAuth): a non-empty SHOPLAZZA_ACCESS_TOKEN skips login state
// entirely and is used verbatim as the store bearer token.
const envAccessToken = "SHOPLAZZA_ACCESS_TOKEN"

// warnWriter picks the factory's stderr handle, falling back to os.Stderr —
// helpers like dashboardClient have no *cobra.Command in scope. Copy of
// cmd/app's warnWriter (private there).
func warnWriter(f *cmdutil.Factory) io.Writer {
	if f.IOStreams.ErrOut != nil {
		return f.IOStreams.ErrOut
	}
	return os.Stderr
}

// resolveStore mirrors cmd/checkout's resolveStore (private there) so every te
// store-side command shares one resolution. override flag > current store; both
// empty → validation. Emptiness is judged AFTER normalization so values like
// "https://" cannot slip through. The current-store fallback goes through
// CurrentStoreDomain() (current profile's domain, legacy field otherwise).
func resolveStore(f *cmdutil.Factory, override string) (string, *output.ExitError) {
	if s := cmdutil.NormalizeStoreDomain(override); s != "" {
		return s, nil
	} else if override != "" {
		return "", output.ErrValidation("invalid --store-domain %q", override)
	}
	if s := cmdutil.NormalizeStoreDomain(f.Config.CurrentStoreDomain()); s != "" {
		return s, nil
	}
	return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no current store selected",
		"run 'shoplazza auth store use --store-domain <domain>' or pass --store-domain")
}

// storeTokenFor returns (token, domain) for override: an override matching a
// profile → that profile's credentials (AccessTokenReadyForProfile, cached/
// persisted); an unmatched override → an ephemeral exchange (te -s ad-hoc,
// v1 semantics, ZERO persistence — never creates a profile or writes a
// token); no override → the current profile's credentials.
func storeTokenFor(ctx context.Context, f *cmdutil.Factory, override string) (string, string, error) {
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	if s := cmdutil.NormalizeStoreDomain(override); s != "" {
		if p := f.Config.FindProfileByStore(s); p != nil {
			tok, err := mgr.AccessTokenReadyForProfile(ctx, f.ConfigPath, *p)
			return tok, s, err
		}
		tok, err := mgr.ExchangeEphemeral(ctx, s) // v1 ad-hoc semantics, ZERO persistence
		return tok, s, err
	}
	p, err := cmdutil.ResolveProfile(f, nil)
	if err != nil {
		return "", "", err
	}
	tok, terr := mgr.AccessTokenReadyForProfile(ctx, f.ConfigPath, *p)
	return tok, p.StoreDomain, terr
}

// requireLogin is the light gate (logged-in, no current store required) — copy
// of cmd/app's requireLogin (private there). SHOPLAZZA_ACCESS_TOKEN bypasses
// the gate, matching cmdutil.RequireAuth's project-wide contract.
func requireLogin(ctx context.Context, f *cmdutil.Factory) error {
	if os.Getenv(envAccessToken) != "" {
		return nil
	}
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	status, err := mgr.CurrentStatus()
	if err != nil {
		return output.ErrAuth("failed to check login status: %v", err)
	}
	if !status.LoggedIn {
		return output.ErrWithHint(output.ExitAuth, output.TypeAuth,
			"not logged in", "run 'shoplazza auth login' to authenticate")
	}
	return nil
}

// storeClient builds a store-openapi/OSS client (store:<domain> token) for
// override (te -s), or the current profile's store when override is empty —
// routed through storeTokenFor, so an override matching a profile uses its
// credentials and an unmatched override mints an ephemeral, unpersisted
// token. Returns the resolved domain alongside the client since several
// callers display it (serve's banner, deploy's "enabled_in"). With
// SHOPLAZZA_ACCESS_TOKEN set, the env token is used directly (no login
// state) — base URL is SHOPLAZZA_CLI_API_BASE_URL when set, else
// https://<domain> (mirrors cmdutil.NewDefaultFactory's wiring).
func storeClient(ctx context.Context, f *cmdutil.Factory, override string) (*client.Client, string, error) {
	domain, sErr := resolveStore(f, override)
	if sErr != nil {
		return nil, "", sErr
	}
	if tok := os.Getenv(envAccessToken); tok != "" {
		base := os.Getenv("SHOPLAZZA_CLI_API_BASE_URL")
		if base == "" {
			base = "https://" + domain
		}
		c := client.New(base)
		c.SetBearerToken(tok)
		return c, domain, nil
	}
	tok, domain, tErr := storeTokenFor(ctx, f, override)
	if tErr != nil {
		return nil, "", storeTokenError(tErr)
	}
	c := client.New("https://" + domain)
	c.SetBearerToken(tok)
	return c, domain, nil
}

// storeTokenError classifies a failed store-token mint. storeTokenFor's "no
// profile configured" / ResolveProfile failures already carry the right
// envelope (type + hint) and pass through unchanged; a non-2xx exchange
// response keeps its server message + status (auth-class with a re-login
// hint, mirroring cmdutil.RequireAuth); a wire failure is network-class
// (exit 3 would misdirect the user to re-login); anything else is a plain
// auth error.
func storeTokenError(err error) *output.ExitError {
	var already *output.ExitError
	if errors.As(err, &already) {
		return already
	}
	const hint = "run 'shoplazza auth login' to re-authenticate"
	var he *client.HTTPError
	if errors.As(err, &he) {
		return output.ErrAPIAuthHint(he.StatusCode, he.Body, hint)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("store access token unavailable: %v", err)
	}
	return output.ErrAuth("store access token unavailable: %v", err)
}

// dashboardClient builds the Partner Dashboard client (partner token). Copy of
// cmd/app's dashboardClient (private there). Stays login-backed even under the
// SHOPLAZZA_ACCESS_TOKEN bypass — that env token is store-scoped and cannot
// authorize Dashboard calls.
func dashboardClient(ctx context.Context, f *cmdutil.Factory) (*app.Dashboard, error) {
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := mgr.PartnerToken()
	if err != nil {
		return nil, output.ErrAuth("failed to read partner token: %v", err)
	}
	if tok == "" {
		return nil, output.ErrWithHint(output.ExitAuth, output.TypeAuth,
			"partner token unavailable", "run 'shoplazza auth login' to re-authenticate")
	}
	c := client.New(f.AuthClient.BaseURL)
	// Best-effort, but surface the failure: without the cli-user-id header the
	// backend later 403s with no visible cause (mirrors cmd/app).
	if uid, uErr := mgr.UserIDReady(ctx); uErr == nil && uid != "" {
		c.Headers["cli-user-id"] = uid
	} else if uErr != nil {
		fmt.Fprintf(warnWriter(f), "warning: could not resolve login user id (Dashboard calls may 403): %v\n", uErr)
	}
	return app.NewDashboard(c, tok), nil
}

// partnerOpenapiClient builds the app-token partner-openapi client (app:<client_id>
// token + app-client-id header). Copy of cmd/app's (private there). Used by
// connect and release.
func partnerOpenapiClient(ctx context.Context, f *cmdutil.Factory, clientID, clientSecret, partnerID, baseURL string) (*client.Client, error) {
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := mgr.AppTokenReady(ctx, clientID, clientSecret, partnerID)
	if err != nil {
		return nil, output.ErrAuth("app token unavailable: %v", err)
	}
	c := client.New(baseURL)
	c.SetBearerToken(tok)
	c.Headers["app-client-id"] = clientID
	return c, nil
}

// apiError maps a client error to the right envelope: HTTP → api (naming the
// failing endpoint; 403→auth inside ErrAPI); transport-level net.Error →
// network; else internal. Mirrors cmd/app's apiError.
func apiError(err error) *output.ExitError {
	var he *client.HTTPError
	if errors.As(err, &he) {
		return output.ErrAPI(he.StatusCode, he.Body, "").WithEndpoint(he.Method, he.Path)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}
