package appcmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"os"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/app/project"
	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

// warnWriter picks the factory's stderr handle, falling back to os.Stderr —
// helpers like dashboardClient have no *cobra.Command in scope.
func warnWriter(f *cmdutil.Factory) io.Writer {
	if f.IOStreams.ErrOut != nil {
		return f.IOStreams.ErrOut
	}
	return os.Stderr
}

// reconcileExtensionApps warns and drops the extension id when a v1 config's
// owning app (AppID) differs from the deploy target, so it's created fresh.
func reconcileExtensionApps(w io.Writer, locals []app.LocalExt, activeClientID string) []app.LocalExt {
	for i := range locals {
		if locals[i].AppID != "" && locals[i].AppID != activeClientID {
			fmt.Fprintf(w, "warning: extension %q was created under app %s; deploying to %s creates it as a NEW extension here (its old id is ignored)\n",
				locals[i].Name, locals[i].AppID, activeClientID)
			locals[i].ExtensionID = ""
		}
	}
	return locals
}

// requireLogin is the light app gate: only verifies "logged in (has UAT)".
// It does NOT require a current store (unlike cmdutil.RequireAuth).
func requireLogin(ctx context.Context, f *cmdutil.Factory) error {
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

// dashboardClient builds the Partner Dashboard client (partner token bearer).
// f.AuthClient already points at the partner/Dashboard host.
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
	// The backend's oauth also keys on a cli-user-id header (the login user_id);
	// without it the backend later 403s with no visible cause. Best-effort, but
	// surface the failure.
	if uid, uErr := mgr.UserIDReady(ctx); uErr == nil && uid != "" {
		c.Headers["cli-user-id"] = uid
	} else if uErr != nil {
		fmt.Fprintf(warnWriter(f), "warning: could not resolve login user id (Dashboard calls may 403): %v\n", uErr)
	}
	// Access-Token carries the current STORE token, forwarded verbatim to
	// downstream store-openapi calls (e.g. generate's theme-version lookup).
	// Best-effort: partner-level commands with no current store just omit it.
	if domain := f.Config.CurrentStoreDomain(); domain != "" {
		if stok, sErr := storeTokenForDomain(ctx, f, mgr, domain); sErr == nil && stok != "" {
			c.SetBearerToken(stok)
		} else if sErr != nil {
			fmt.Fprintf(warnWriter(f), "warning: could not mint a store token for %s (store-scoped Dashboard calls may 403): %v\n",
				domain, sErr)
		}
	}
	return app.NewDashboard(c, tok), nil
}

// storeTokenForDomain mints a store token for domain: a profile bound to it
// uses AccessTokenReadyForProfile (cached/persisted credentials); otherwise
// an ephemeral, unpersisted exchange (mirrors theme_extension's storeTokenFor
// ad-hoc path — a legacy-only current store with no matching profile yet).
func storeTokenForDomain(ctx context.Context, f *cmdutil.Factory, mgr *internalauth.Manager, domain string) (string, error) {
	if p := f.Config.FindProfileByStore(domain); p != nil {
		return mgr.AccessTokenReadyForProfile(ctx, f.ConfigPath, *p)
	}
	return mgr.ExchangeEphemeral(ctx, domain)
}

// storeClient builds a store-openapi/OSS client (store:<domain> token) for the
// resolved target store.
func storeClient(ctx context.Context, f *cmdutil.Factory, storeDomain string) (*client.Client, error) {
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := storeTokenForDomain(ctx, f, mgr, storeDomain)
	if err != nil {
		// A token mint that died on the wire is a network problem, not an auth
		// one — exit 3 would misdirect the user to re-login.
		var netErr net.Error
		if errors.As(err, &netErr) {
			return nil, output.ErrNetwork("store access token unavailable: %v", err)
		}
		return nil, output.ErrAuth("store access token unavailable: %v", err)
	}
	c := client.New("https://" + storeDomain)
	c.SetBearerToken(tok)
	return c, nil
}

// partnerOpenapiClient builds a partner-openapi client (app:<client_id> token +
// app-client-id header) for functions / theme-extensions connection (2025-06).
func partnerOpenapiClient(ctx context.Context, f *cmdutil.Factory, clientID, clientSecret, partnerID, baseURL string) (*client.Client, error) {
	mgr := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient)
	tok, err := mgr.AppTokenReady(ctx, clientID, clientSecret, partnerID)
	if err != nil {
		// Same classification as storeClient: wire failures are network-class.
		var netErr net.Error
		if errors.As(err, &netErr) {
			return nil, output.ErrNetwork("app token unavailable: %v", err)
		}
		return nil, output.ErrAuth("app token unavailable: %v", err)
	}
	c := client.New(baseURL)
	c.SetBearerToken(tok) // partner-openapi auth: app token via access-token header
	// The app-client-id header carries the client_id.
	c.Headers["app-client-id"] = clientID
	return c, nil
}

// openProject resolves and opens the app project rooted at path (relative to
// cwd or absolute). Used by project-level commands that accept a --path flag.
func openProject(path string) (*project.Project, error) {
	cwd, err := os.Getwd()
	if err != nil {
		return nil, output.ErrInternal("cannot resolve cwd: %v", err)
	}
	return project.Open(project.Resolve(cwd, path))
}

// resolveTargetStore resolves the target store for deploy/dev: the current store
// (config.json.store_domain), which empty is a validation error. deploy/dev no
// longer take a --store-domain override — they always target the current store.
func resolveTargetStore(current string) (string, error) {
	if current != "" {
		return current, nil
	}
	return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
		"no target store",
		"run 'shoplazza auth store use --store-domain <domain>' to set the current store")
}

// apiError maps a Dashboard/client error to the right envelope: a non-2xx HTTP
// response (incl. 403->auth, 5xx masking, status_code/request_id) goes through
// output.ErrAPI (naming the failing endpoint); a transport-level net.Error is
// network-class; anything else is a genuine client-side fault (internal).
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

// resolveStoreID resolves the numeric store id for targetStore (sent as
// ?store_id on version/generate — without it the backend defaults to store 0
// and 500s). A hard StoreIDFor error is surfaced via apiError; an EMPTY id with
// a nil error (StoreIDFor returns ("", nil) when the session has no UAT or the
// refresh didn't capture a store_id) is treated as the same resolution failure
// rather than being passed through to a confusing backend 500.
func resolveStoreID(ctx context.Context, f *cmdutil.Factory, targetStore string) (string, error) {
	storeID, err := internalauth.NewManager(f.Config, f.ConfigPath, f.AuthClient).StoreIDFor(ctx, targetStore)
	if err != nil {
		return "", apiError(err).WithHint("could not resolve store id for " + targetStore + " — run 'shoplazza auth login' if your session expired")
	}
	if storeID == "" {
		return "", output.ErrWithHint(output.ExitAuth, output.TypeAuth,
			"could not resolve store id for "+targetStore,
			"run 'shoplazza auth login' if your session expired")
	}
	return storeID, nil
}

// appRef is the resolved identity of an app for init/config-link, including the
// owning partner so callers can persist it into the app config.
type appRef struct {
	ClientID  string
	PartnerID string
	Scopes    []string
	Name      string
}

// resolveAppRef resolves the target app: either create a new app (init: --name;
// config link: --create --name, under partnerFlag) or link an existing one
// (--client-id). Shared by `app init` and `app config link`. The returned appRef
// carries PartnerID so the caller can write it into the app config (partner↔app
// is immutable).
//
//   - create: needs a partner to create under (GetPartners + selectPartner,
//     honoring --partner); the created app inherits that partner.
//   - link: the partner is derived FROM the client_id via /info (keyed on
//     client_id alone) — no --partner needed, correct even for accounts with
//     multiple partners. Scopes/name still come from GetAppConfig.
func resolveAppRef(ctx context.Context, d *app.Dashboard, clientID string, create bool, name, partnerFlag string) (appRef, error) {
	if !create && clientID == "" {
		return appRef{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"specify --client-id <id> to link an existing app, or --create to make a new one",
			"run 'shoplazza app list' to find a client_id")
	}
	if create {
		partners, err := d.GetPartners(ctx)
		if err != nil {
			return appRef{}, apiError(err)
		}
		pid, err := selectPartner(partners.Partners, partnerFlag)
		if err != nil {
			return appRef{}, err
		}
		if name == "" {
			return appRef{}, output.ErrValidation("--create requires --name")
		}
		created, err := d.CreateApp(ctx, pid, name)
		if err != nil {
			return appRef{}, apiError(err)
		}
		return appRef{ClientID: created.ClientID, PartnerID: pid, Scopes: created.Scopes, Name: created.Name}, nil
	}
	// Link existing: derive the owning partner from the client_id.
	info, err := d.GetCompleteInfo(ctx, clientID)
	if err != nil {
		return appRef{}, apiError(err)
	}
	pid := string(info.Partner.ID)
	if pid == "" {
		return appRef{}, output.ErrInternal("the app's partner could not be resolved from /info for client_id %s", clientID)
	}
	cfg, err := d.GetAppConfig(ctx, pid, clientID)
	if err != nil {
		return appRef{}, apiError(err)
	}
	return appRef{ClientID: cfg.ClientID, PartnerID: pid, Scopes: cfg.Scopes, Name: cfg.Name}, nil
}

// activeAppConfig reads the project's active config and validates that client_id
// is present (network-free, so a bad project fails before any auth). The partner
// id is NOT required here — it may be absent in v1-scaffolded projects; callers
// resolve it via ensurePartnerID once a Dashboard client is in hand. Used by the
// read commands (dev/deploy/function) now that partner is sourced from the config
// rather than a --partner flag.
func activeAppConfig(p *project.Project) (project.Config, *output.ExitError) {
	cfg, err := p.ActiveConfig()
	if err != nil {
		return project.Config{}, output.ErrValidation("cannot read active config: %v", err)
	}
	if cfg.ClientID == "" {
		return project.Config{}, output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no client_id in active config", "run 'shoplazza app config link' or 'shoplazza app config use'")
	}
	return cfg, nil
}

// ensurePartnerID returns the config's partner_id, resolving it live from /info
// (keyed on client_id alone) when the toml lacks it. v1-scaffolded projects only
// ever write client_id + scopes, never partner_id — this resolves it the same way
// `app config link` and v1 do, so those projects dev/deploy/release without a
// re-link. A real v2-linked project has partner_id in the toml and skips the call.
func ensurePartnerID(ctx context.Context, d *app.Dashboard, cfg project.Config) (string, *output.ExitError) {
	if cfg.PartnerID != "" {
		return cfg.PartnerID, nil
	}
	info, err := d.GetCompleteInfo(ctx, cfg.ClientID)
	if err != nil {
		return "", apiError(err)
	}
	pid := string(info.Partner.ID)
	if pid == "" {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"no partner_id in active config, and /info resolved none for client_id "+cfg.ClientID,
			"re-run 'shoplazza app config link' to populate it")
	}
	return pid, nil
}
