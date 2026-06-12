package app

import (
	"encoding/json"
	"strings"
)

// flexStr decodes a JSON string OR number into a string. The Dashboard returns
// some ids (e.g. partner id) as JSON numbers though the v1-derived contract
// assumed strings — tolerate both (real shape ≠ assumed shape).
type flexStr string

func (s *flexStr) UnmarshalJSON(b []byte) error {
	if len(b) == 0 || string(b) == "null" {
		return nil
	}
	if b[0] == '"' {
		var str string
		if err := json.Unmarshal(b, &str); err != nil {
			return err
		}
		*s = flexStr(str)
		return nil
	}
	*s = flexStr(strings.TrimSpace(string(b))) // raw JSON number token
	return nil
}

type Partner struct {
	ID flexStr `json:"id"`
	// The Dashboard returns the partner's display name as "business_name" on both
	// /partners and /info (the app name uses "name", but the partner name does
	// not). Use the raw field the API actually sends, in and out.
	BusinessName string `json:"business_name"`
}
type App struct {
	ID       flexStr  `json:"id"` // internal numeric app id (apps-list endpoint)
	ClientID string   `json:"client_id"`
	Name     string   `json:"name"`
	Scopes   []string `json:"scopes"`
}
type AppConfig struct {
	ClientID     string   `json:"client_id"`
	Name         string   `json:"name"`
	Scopes       []string `json:"scopes"`
	ClientSecret string   `json:"client_secret"` // endpoint 5 only — never persisted
	PartnerID    string   `json:"partner_id"`    // endpoint 5 only
}
type Extension struct {
	ExtensionID      string `json:"extension_id"`
	ExtensionName    string `json:"extension_name"`
	ExtensionType    string `json:"extension_type"` // theme|checkout|function
	ExtensionVersion string `json:"extension_version"`
	// ExtensionVersionID is flexStr: the Dashboard's extension_versions /
	// version/generate responses return it as a JSON NUMBER, while functions/
	// create returns it as a string. flexStr decodes either and re-marshals as a
	// quoted string for the deploy body (the shape that /deploy accepts).
	ExtensionVersionID flexStr `json:"extension_version_id"`
	ResourceURL        string  `json:"resource_url"`
	// Exts is the version description sent to the version-task body as `exts`
	// (v1 field name). json:"-" — the version-task/PUT bodies are hand-built maps,
	// so this is a pure internal carrier and never marshals into any request body.
	Exts string `json:"-"`
}

type PartnersResp struct {
	Partners []Partner `json:"partners"`
}
type AppsResp struct {
	Apps  []App `json:"apps"`
	Total int   `json:"total"`
}
type TemplateResp struct {
	TemplateType string `json:"template_type"`
	HTTPS        string `json:"https"`
}
type CompleteInfo struct {
	User    map[string]any `json:"user"`
	Partner Partner        `json:"partner"`
	App     App            `json:"app"`
}
type ExtensionsResp struct {
	Extensions []Extension `json:"extensions"`
}
type DeployResp struct {
	Version string `json:"version"`
	Name    string `json:"name"`
}
type GenerateVersionResp struct {
	AppVersion string      `json:"app_version"`
	Extensions []Extension `json:"extensions"`
}
type VersionItem struct {
	ID      int    `json:"id"`
	Version string `json:"version"`
}
type VersionsResp struct {
	Versions []VersionItem `json:"versions"`
	HasMore  bool          `json:"has_more"`
}
