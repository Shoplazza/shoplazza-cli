package app

import (
	"context"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

type checkoutUpsertResp struct {
	Extension extInfo `json:"extension"`
	Data      struct {
		Extension extInfo `json:"extension"`
	} `json:"data"`
}

type extInfo struct {
	ExtensionID string `json:"extension_id"`
	ID          string `json:"id"`
	Name        string `json:"name"`
}

// upsertCheckout creates (existingID == "") or commits (existingID set) a
// checkout extension via store-openapi, returning the extension_id + version id.
// Mirrors cmd/checkout/push.go's create/commit. inner is the pre-built
// {resource_url, version, name, ...} payload (built by the deploy orchestrator).
//
// PostJSON is used rather than DoRaw: doJSON calls unmarshalUnwrapped, which
// only strips the Shoplazza envelope when code=="Success" or ok==true. The
// checkout_extensions response {data:{extension:{...}}, status:"ok"} has no
// such field, so the full body lands in resp — meaning resp.Data.Extension is
// populated. The dual-shape struct covers both that case and any future
// envelope-unwrapped variant where resp.Extension would be populated instead.
func upsertCheckout(ctx context.Context, c *client.Client, inner map[string]any, existingID string) (string, string, *output.ExitError) {
	path := "/openapi/checkout_extensions/create"
	if existingID != "" {
		inner["extension_id"] = existingID
		path = "/openapi/checkout_extensions/commit"
	}
	var resp checkoutUpsertResp
	if err := c.PostJSON(ctx, path, map[string]any{"extension": inner}, &resp); err != nil {
		return "", "", apiOrInternal(err)
	}
	ext := resp.Extension
	if ext.ExtensionID == "" && ext.ID == "" {
		ext = resp.Data.Extension
	}
	if ext.ExtensionID == "" && ext.ID == "" {
		return "", "", output.ErrInternal("checkout upsert (%s) returned no extension_id — unexpected response body", path)
	}
	return ext.ExtensionID, ext.ID, nil
}
