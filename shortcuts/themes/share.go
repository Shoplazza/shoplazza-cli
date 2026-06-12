package themes

import (
	"context"
	"encoding/json"
	"fmt"
	"os"

	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/theme"
	"shoplazza-cli-v2/internal/theme/pack"
	"shoplazza-cli-v2/shortcuts/common"
)

// shareShortcut is the `themes share` workflow: it packages the current
// directory and uploads it to the v1 /openapi/2020-07/themes/upload endpoint
// as a NEW, unpublished theme (server-assigned name), then prints a preview
// URL merchants can open against the share-target store. It is a frozen
// snapshot — share never touches an existing theme.
//
// API choice: BOTH steps stay on the v1 path
// tree — /openapi/2020-07/shop and /openapi/2020-07/themes/upload — for
// byte-exact parity with the v1 CLI's share workflow. The v2 spec exposes
// `/openapi/2026-01/shop`, but share keeps the legacy path so existing
// share-link recipients (often non-CLI tools that scrape the URL shape)
// don't break.
//
// There is no /themes/{id}/share endpoint: the upload hits /themes/upload with
// no theme_id (a fresh temporary slot). Overwriting an existing theme is
// `themes push`'s job, not share's, so a preview can never clobber a real
// theme.
//
// theme_id resolution: the upload endpoint may echo theme_id synchronously or
// return only a task_id for an async job; in the async case we poll the task
// and read theme_id from its info payload, otherwise the preview link would be
// empty.
var shareShortcut = common.Shortcut{
	Service: "themes",
	Command: "share",
	Use:     "share",
	Short:   "Upload the current theme as a new temporary preview and print a shareable link",
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}

		shopPlan := PlanShareShop()               // v1 /openapi/2020-07/shop
		uploadPlan := PlanShareUpload("", "", "") // placeholders for dry-run rendering

		// Dry-run: emit BOTH planned requests so users see the exact URLs
		// that will fire. No filesystem reads beyond cwd resolution; even
		// the settings_schema.json lookup is skipped so dry-run doesn't
		// fail in directories without a theme.
		if in.DryRun {
			name, version, _ := readThemeInfo(cwd)
			if name == "" {
				name = "<theme>"
			}
			if version == "" {
				version = "<version>"
			}
			uploadPlan = PlanShareUpload("", name, version)
			return common.ExecResult{Plans: []common.PlannedRequest{shopPlan, uploadPlan}}, nil
		}

		// Live mode: read theme metadata (required — pack needs the name).
		name, version, err := readThemeInfo(cwd)
		if err != nil {
			return common.ExecResult{}, err
		}
		uploadPlan = PlanShareUpload("", name, version)

		// Step 1: GET /shop — fetch the merchant's primary domain that the
		// preview URL is anchored at (the API base URL is the gateway, not the
		// storefront). Progress goes to stderr so the result JSON on stdout
		// stays pipe-clean.
		prog := output.NewProgress(os.Stderr)
		shopStep := prog.Begin("[share] fetching shop info")
		shopResp, err := common.Send(ctx, in.Client, shopPlan)
		if err != nil {
			shopStep.Fail()
			return common.ExecResult{}, classifyHTTPErr(err, "")
		}
		storeDomain := extractStoreDomain(shopResp)
		shopStep.Done()

		// Step 2: pack cwd into a tmp zip. Deferred cleanup runs on success
		// AND failure — the artifact has no diagnostic value post-failure
		// (the user can rebuild it from cwd) and we want the tmp dir clean.
		pkgStep := prog.Begin("[share] packaging theme files")
		zipName := themeZipName(name, version)
		zipPath, err := pack.Pack(cwd, zipName, pack.PackOptions{})
		if err != nil {
			pkgStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO("pack zip", err)
		}
		defer os.Remove(zipPath)
		pkgStep.Done()

		// Step 3: multipart upload (v1 path) + theme_id resolution via the
		// shared uploadZipResolveThemeID helper (devtheme.go), which polls the
		// task when the endpoint returns only a task_id. No fallback id: share
		// always creates a fresh theme, so an unresolved id is a hard error.
		upStep := prog.Begin("[share] uploading and processing theme")
		returnedThemeID, err := uploadZipResolveThemeID(ctx, in.Client, uploadPlan, zipPath, "")
		if err != nil {
			upStep.Fail()
			return common.ExecResult{}, err
		}
		if returnedThemeID == "" {
			// Server neither echoed a theme_id nor ran an async task: without an
			// id the preview URL is broken ("?preview_theme_id="). Mirror
			// createDevTheme's contract-violation error instead of reporting a
			// hollow success.
			upStep.Fail()
			return common.ExecResult{}, theme.ErrValidation(
				"server did not return a theme id for the share upload; " +
					"cannot build a preview URL — retry")
		}
		upStep.Done()

		previewURL := fmt.Sprintf("https://%s/?preview_theme_id=%s", storeDomain, returnedThemeID)
		return common.ExecResult{Body: map[string]any{
			"preview_url":  previewURL,
			"theme_id":     returnedThemeID,
			"store_domain": storeDomain,
		}}, nil
	},
}

// extractStoreDomain walks the envelope shapes the v1 /shop endpoint has
// shipped — root, root.shop, root.data, root.data.shop — and returns the first
// non-empty "domain" or "store_domain" field. Returns "" if no shape matches;
// the caller then produces a degraded but still-parseable preview URL rather
// than failing, since the upload itself succeeded.
func extractStoreDomain(resp map[string]any) string {
	for _, m := range []map[string]any{
		resp,
		mapField(resp, "shop"),                   // unwrapped (ok:true) v1 shape: {shop:{domain}}
		mapField(resp, "data"),                   // wrapped (no-ok) shape: {data:{...}}
		mapField(mapField(resp, "data"), "shop"), // wrapped v1 shape: {data:{shop:{domain}}}
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

// asMap coerces an `any` (the type of client.RawResponse.Body) into a
// map[string]any when possible. Returns nil for non-map values so the
// downstream mapField / extractStringField calls degrade gracefully
// instead of panicking on a type assertion.
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return nil
}

// themeIDFromTask extracts the share-ready theme_id from a completed upload
// task. The id may sit directly on the task, or (v1 shape) inside the "info"
// field — a JSON string like {"name":"...","theme_id":"..."}.
func themeIDFromTask(task map[string]any) string {
	if id := getString(task, "theme_id"); id != "" {
		return id
	}
	if info := getString(task, "info"); info != "" {
		var parsed struct {
			ThemeID string `json:"theme_id"`
		}
		if err := json.Unmarshal([]byte(info), &parsed); err == nil && parsed.ThemeID != "" {
			return parsed.ThemeID
		}
	}
	return ""
}
