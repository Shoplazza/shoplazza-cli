package themes

import (
	"context"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// Response-shape helpers shared by the page/block commands. They used to live
// in the workflow files (pull/push/share/serve) that moved to cmd/theme.

// mapField returns m[k] as a map[string]any, or nil. Nil-safe input.
func mapField(m map[string]any, k string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[k].(map[string]any); ok {
		return v
	}
	return nil
}

// getString returns m[k] as string, or "" if absent or wrong type.
func getString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// asMap coerces an `any` (the type of client.RawResponse.Body) into a
// map[string]any when possible. Returns nil for non-map values so the
// downstream mapField / getString calls degrade gracefully instead of
// panicking on a type assertion.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// extractStoreDomainBest fetches the shop's primary domain for preview URLs.
// Best-effort: errors degrade to "<unknown-shop>" so the URL still prints.
func extractStoreDomainBest(ctx context.Context, c *client.Client) string {
	resp, err := common.Send(ctx, c, PlanShop())
	if err != nil {
		return "<unknown-shop>"
	}
	d := extractStoreDomain(resp)
	if d == "" {
		return "<unknown-shop>"
	}
	return d
}

// extractStoreDomain digs the shop domain out of the response envelope shapes
// (root, root.shop, root.data, root.data.shop), preferring "domain" then the
// v1 "store_domain" alias.
func extractStoreDomain(resp map[string]any) string {
	for _, m := range []map[string]any{
		resp,
		mapField(resp, "shop"),
		mapField(resp, "data"),
		mapField(mapField(resp, "data"), "shop"),
	} {
		if m == nil {
			continue
		}
		if d, ok := m["domain"].(string); ok && d != "" {
			return d
		}
		if d, ok := m["store_domain"].(string); ok && d != "" {
			return d
		}
	}
	return ""
}
