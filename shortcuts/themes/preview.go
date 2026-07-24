package themes

import (
	"context"
	"fmt"
	"net/url"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// previewShortcut prints a storefront preview URL for a theme (optionally an
// edit session). The domain comes from the resolved client base URL, so it
// makes no API call; uploading is +share's job.
var previewShortcut = common.Shortcut{
	Service: "themes",
	Command: "+preview",
	Use:     "+preview",
	Short:   "Print a storefront preview URL for a theme (optionally an edit session)",
	Flags: []common.Flag{
		{
			Name:        "theme-id",
			Short:       "t",
			Type:        common.FlagString,
			Required:    true,
			Description: "Theme ID to preview.",
		},
		{
			Name:        "oseid",
			Type:        common.FlagString,
			Description: "Optional edit session id; preview the temp session state.",
		},
		{
			Name:        "path",
			Type:        common.FlagString,
			Default:     "/",
			Description: "Storefront path to preview, e.g. /products/xxx. Defaults to /.",
		},
		{
			Name:        "locale",
			Type:        common.FlagString,
			Description: "Optional locale, e.g. zh_CN / en_US.",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		themeID := in.Flags.GetString("theme-id")
		oseid := in.Flags.GetString("oseid")
		path := in.Flags.GetString("path")
		locale := in.Flags.GetString("locale")

		baseURL := ""
		if in.Client != nil {
			baseURL = in.Client.BaseURL
		}
		domain := storeDomainFromBaseURL(baseURL)
		previewURL := buildPreviewURL(domain, path, themeID, oseid, locale)

		return common.ExecResult{Body: map[string]any{
			"preview_url":  previewURL,
			"theme_id":     themeID,
			"oseid":        oseid,
			"store_domain": domain,
		}}, nil
	},
}

// storeDomainFromBaseURL returns the host of the client base URL (the active
// profile's store domain).
func storeDomainFromBaseURL(baseURL string) string {
	if baseURL == "" {
		return ""
	}
	if u, err := url.Parse(baseURL); err == nil && u.Host != "" {
		return u.Host
	}
	return strings.TrimPrefix(strings.TrimPrefix(baseURL, "https://"), "http://")
}

// buildPreviewURL assembles the storefront preview URL from its parts.
func buildPreviewURL(domain, path, themeID, oseid, locale string) string {
	if path == "" {
		path = "/"
	}
	if !strings.HasPrefix(path, "/") {
		path = "/" + path
	}

	if oseid == "" {
		u := fmt.Sprintf("https://%s%s?preview_theme_id=%s", domain, path, url.QueryEscape(themeID))
		if locale != "" {
			u += "&locale=" + url.QueryEscape(locale)
		}
		return u
	}

	if locale == "" {
		locale = "en_US"
	}
	return fmt.Sprintf("https://%s%s?%d&oseid=%s&preview_theme_id=%s&locale=%s&st=",
		domain, path, time.Now().UnixMilli(),
		url.QueryEscape(oseid), url.QueryEscape(themeID), url.QueryEscape(locale))
}
