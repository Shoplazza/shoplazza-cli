package app

import (
	"context"
	"fmt"
	"strconv"

	"shoplazza-cli-v2/internal/client"
)

// Dashboard is the typed client for Partner Dashboard /api/cli/v2 endpoints.
// Bearer = partner token. The caller supplies a client already pointed at the
// Dashboard host.
type Dashboard struct {
	c *client.Client
}

func NewDashboard(c *client.Client, partnerToken string) *Dashboard {
	// The /api/cli/v2 oauth reads the PARTNER token from the Cli-Partner-Token
	// header — NOT Access-Token. Access-Token is left for the caller to set to
	// the target STORE token, which the backend forwards verbatim to downstream
	// store-openapi calls (e.g. generate's theme-version lookup, which only the
	// store token is authorized for). This mirrors v1, whose Dashboard calls sent
	// the store access_token as access-token.
	c.Headers["Cli-Partner-Token"] = partnerToken
	// NOTE: the backend's oauth also requires a per-session cli-user-id header;
	// it's set by the caller (cmd/app.dashboardClient) where the session loads.
	return &Dashboard{c: c}
}

const base = "/api/cli/v2"

// 1
func (d *Dashboard) GetPartners(ctx context.Context) (PartnersResp, error) {
	var out PartnersResp
	return out, d.c.GetJSON(ctx, base+"/partners", &out)
}

// 2
func (d *Dashboard) GetApps(ctx context.Context, partnerID string) (AppsResp, error) {
	var out AppsResp
	return out, d.c.GetJSON(ctx, fmt.Sprintf("%s/partners/%s/apps", base, partnerID), &out)
}

// appWrap is the single-app response envelope. The Dashboard nests create /
// get-config under "app" (NOT the apps-LIST elements, which carry client_id at
// the element root). The secret field is "secret" (not "client_secret"); the
// response carries no partner_id (the caller already knows it).
type appWrap struct {
	App struct {
		ClientID string   `json:"client_id"`
		ID       int64    `json:"id"` // internal numeric app id (unused; here for completeness)
		Name     string   `json:"name"`
		Secret   string   `json:"secret"`
		Scopes   []string `json:"scopes"`
	} `json:"app"`
}

// 3 — v1 parity: createApp sends only app_name (no app_type).
func (d *Dashboard) CreateApp(ctx context.Context, partnerID, appName string) (App, error) {
	var w appWrap
	body := map[string]any{"app_name": appName}
	if err := d.c.PostJSON(ctx, fmt.Sprintf("%s/partners/%s/apps", base, partnerID), body, &w); err != nil {
		return App{}, err
	}
	return App{ClientID: w.App.ClientID, Name: w.App.Name, Scopes: w.App.Scopes}, nil
}

// 4
func (d *Dashboard) GetTemplate(ctx context.Context, templateType string) (TemplateResp, error) {
	var out TemplateResp
	return out, d.c.GetJSONWithQuery(ctx, base+"/template", map[string]any{"template_type": templateType}, &out)
}

// 5 — returns the app under "app" with its secret (field "secret"); never
// persist the secret. partner_id isn't in the response, so carry the caller's.
func (d *Dashboard) GetAppConfig(ctx context.Context, partnerID, clientID string) (AppConfig, error) {
	var w appWrap
	p := fmt.Sprintf("%s/partners/%s/apps/%s", base, partnerID, clientID)
	if err := d.c.GetJSON(ctx, p, &w); err != nil {
		return AppConfig{}, err
	}
	return AppConfig{
		ClientID:     w.App.ClientID,
		Name:         w.App.Name,
		Scopes:       w.App.Scopes,
		ClientSecret: w.App.Secret,
		PartnerID:    partnerID,
	}, nil
}

// 6
func (d *Dashboard) GetCompleteInfo(ctx context.Context, clientID string) (CompleteInfo, error) {
	var out CompleteInfo
	return out, d.c.GetJSONWithQuery(ctx, base+"/info", map[string]any{"app_client_id": clientID}, &out)
}

// (7 post_policy — v1 dead code, intentionally NOT implemented.)

// 8
func (d *Dashboard) ExtensionDev(ctx context.Context, partnerID, clientID, storeID string, appPayload any) (map[string]any, error) {
	var out map[string]any
	p := fmt.Sprintf("%s/partners/%s/apps/%s/dev", base, partnerID, clientID)
	return out, d.c.PostJSON(ctx, p, deployBody(appPayload, storeID), &out)
}

// 9
func (d *Dashboard) ExtensionDeploy(ctx context.Context, partnerID, clientID, storeID string, appPayload any) (DeployResp, error) {
	var out DeployResp
	p := fmt.Sprintf("%s/partners/%s/apps/%s/deploy", base, partnerID, clientID)
	return out, d.c.PostJSON(ctx, p, deployBody(appPayload, storeID), &out)
}

// deployBody builds the /deploy and /dev request body. v1 (cli.js
// extensionDeploy/extensionDev) sends `{...data, store_id}` — i.e. the app
// payload wrapped in `app` PLUS a top-level `store_id` sibling. The backend
// resolves the target store from this store_id; omitting it yields a
// `ResourceNotFound` 404, and sending it as a string yields a 400 (the
// backend's AppDeployForm.store_id is uint64). So send it as a JSON number,
// matching v1 (which carries the backend-returned numeric store_id verbatim).
// An empty/non-numeric store_id is omitted (the backend then 404s — the
// genuine "no target store" case).
func deployBody(appPayload any, storeID string) map[string]any {
	body := map[string]any{"app": appPayload}
	if n, err := strconv.ParseUint(storeID, 10, 64); err == nil {
		body["store_id"] = n
	}
	return body
}

// 10
func (d *Dashboard) GetExtensionVersions(ctx context.Context, partnerID, clientID string, q map[string]any) (ExtensionsResp, error) {
	var out ExtensionsResp
	p := fmt.Sprintf("%s/partners/%s/apps/%s/extension_versions", base, partnerID, clientID)
	return out, d.c.GetJSONWithQuery(ctx, p, q, &out)
}

// 11 — needs ?store_id so the backend can resolve/create the (dev) store;
// without it the backend defaults to store 0 and 500s (v1 sent store_id too).
func (d *Dashboard) GenerateVersion(ctx context.Context, partnerID, clientID, isDev, storeID string) (GenerateVersionResp, error) {
	var out GenerateVersionResp
	p := fmt.Sprintf("%s/partners/%s/apps/%s/version/generate", base, partnerID, clientID)
	q := map[string]any{"is_dev": isDev}
	if storeID != "" {
		q["store_id"] = storeID
	}
	return out, d.c.GetJSONWithQuery(ctx, p, q, &out)
}

// 12
func (d *Dashboard) GetVersions(ctx context.Context, partnerID, clientID string, offset, limit int) (VersionsResp, error) {
	var out VersionsResp
	p := fmt.Sprintf("%s/partners/%s/apps/%s/versions", base, partnerID, clientID)
	q := map[string]any{"offset": offset, "limit": limit}
	return out, d.c.GetJSONWithQuery(ctx, p, q, &out)
}
