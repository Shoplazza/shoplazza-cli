package theme_extension

import (
	"context"
	"encoding/json"
	"strconv"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

// themeOptions lists the current store's themes for a fuzzy --theme-id picker.
// serve has no --store-domain, so it uses the current profile's store. On any
// error (or an empty list) the caller (ResolveFlags) degrades to manual entry —
// this must never harden into a hard failure.
func themeOptions(ctx context.Context, _ *cobra.Command, f *cmdutil.Factory) ([]interact.Option, error) {
	store, _, err := storeClient(ctx, f, "")
	if err != nil {
		return nil, err
	}
	themes, exErr := te.ListThemes(ctx, store)
	if exErr != nil {
		return nil, exErr
	}
	return themePickerOptions(themes), nil
}

// themePickerOptions maps a store themes list to picker options: value = theme
// id, label = title/name (plus role when present). Rows without an id are
// skipped; an empty title falls back to the id. Pure, so the mapping is
// unit-tested against the shapes the id/title fields can take.
func themePickerOptions(themes []map[string]any) []interact.Option {
	opts := make([]interact.Option, 0, len(themes))
	for _, t := range themes {
		id := firstThemeString(t, "id", "theme_id")
		if id == "" {
			continue
		}
		label := firstThemeString(t, "title", "name")
		if label == "" {
			label = id
		}
		if role := firstThemeString(t, "role", "theme_type"); role != "" {
			label += " · " + role
		}
		opts = append(opts, interact.Option{Label: label, Value: id})
	}
	return opts
}

// appClientIDOptions lists the account's apps across all partners for a fuzzy
// --client-id picker (value = client_id, label = name + client_id). Uses the
// Partner Dashboard (partner token). On any error / empty list the caller
// (ResolveFlags) degrades to manual entry — never a hard failure.
func appClientIDOptions(ctx context.Context, _ *cobra.Command, f *cmdutil.Factory) ([]interact.Option, error) {
	d, err := dashboardClient(ctx, f)
	if err != nil {
		return nil, err
	}
	partners, err := d.GetPartners(ctx)
	if err != nil {
		return nil, err
	}
	var opts []interact.Option
	for _, p := range partners.Partners {
		apps, aErr := d.GetApps(ctx, string(p.ID))
		if aErr != nil {
			return nil, aErr
		}
		opts = append(opts, appPickerOptions(apps.Apps)...)
	}
	return opts, nil
}

// appPickerOptions maps a partner's apps to picker options: value = client_id,
// label = name (plus client_id), skipping apps with no client_id. Pure, so the
// mapping is unit-tested.
func appPickerOptions(apps []app.App) []interact.Option {
	opts := make([]interact.Option, 0, len(apps))
	for _, a := range apps {
		if a.ClientID == "" {
			continue
		}
		label := a.Name
		if label == "" {
			label = a.ClientID
		} else {
			label += " · " + a.ClientID
		}
		opts = append(opts, interact.Option{Label: label, Value: a.ClientID})
	}
	return opts
}

// firstThemeString returns the first non-empty value among keys, coercing the
// forms a JSON decode yields for an id/title (string, json.Number, float64).
func firstThemeString(m map[string]any, keys ...string) string {
	for _, k := range keys {
		switch v := m[k].(type) {
		case string:
			if v != "" {
				return v
			}
		case json.Number:
			return v.String()
		case float64:
			return strconv.FormatFloat(v, 'f', -1, 64)
		}
	}
	return ""
}
