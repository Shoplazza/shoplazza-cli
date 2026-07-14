// Package themes provides shortcut workflow commands for the themes resource.
//
// The dynamic CRUD commands (themes list/get/publish/delete/...) are registered
// separately by the dynamic engine from the v2 spec. This package only adds
// the workflow commands (init / package / pull / push / share / serve /
// +preview) that need multi-step orchestration or compress parameters.
//
// API version mix:
//   - v2 spec endpoints (16) — `/openapi/2026-01/themes/...` (including task polling)
//   - v1 path endpoints (spec missing) — `/openapi/2020-07/themes/{upload,download}`
//   - share keeps the entire v1 path tree for byte-exact parity with the v1 CLI.
package themes

import (
	"shoplazza-cli-v2/shortcuts/common"
)

const (
	themeBaseV202601 = common.APIPrefix + "/themes"
	themeBaseV1      = "/openapi/2020-07/themes"
	shopV202601      = common.APIPrefix + "/shop"
	shopV1           = "/openapi/2020-07/shop"
)

// ─────────── v2 spec endpoints (dynamic engine already registers these) ───────────

// PlanDetail describes GET /themes/{id}.
func PlanDetail(themeID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: themeBaseV202601 + "/" + themeID}
}

// PlanPublish describes PATCH /themes/{id}/publish (set as merchant default).
func PlanPublish(themeID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "PATCH", Path: themeBaseV202601 + "/" + themeID + "/publish"}
}

// PlanDelete describes DELETE /themes/{id}.
func PlanDelete(themeID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "DELETE", Path: themeBaseV202601 + "/" + themeID}
}

// PlanTaskDetail describes GET /themes/task/{taskID} (async task polling).
// This is v2 spec — the dynamic `themes task` command maps to the same path.
func PlanTaskDetail(taskID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: themeBaseV202601 + "/task/" + taskID}
}

// PlanDocTree describes GET /themes/{id}/doctree (full file tree snapshot).
func PlanDocTree(themeID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: themeBaseV202601 + "/" + themeID + "/doctree"}
}

// PlanDocCreate describes POST /themes/{id}/doc (add or replace a single file).
// The server's CreateThemeFileRequest requires the file fields under a "doc"
// object, so the caller's {type,location,content} map is wrapped accordingly.
func PlanDocCreate(themeID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: themeBaseV202601 + "/" + themeID + "/doc", Body: map[string]any{"doc": body}}
}

// PlanDocPatch describes PATCH /themes/{id}/doc (in-place edit a single file).
// Like create, the server's UpdateThemeFileRequest requires a "doc" wrapper.
func PlanDocPatch(themeID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PATCH", Path: themeBaseV202601 + "/" + themeID + "/doc", Body: map[string]any{"doc": body}}
}

// PlanDocDelete describes DELETE /themes/{id}/doc?type=...&location=... (remove a file).
func PlanDocDelete(themeID string, query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "DELETE", Path: themeBaseV202601 + "/" + themeID + "/doc", Query: query}
}

// PlanShop describes GET /shop (the merchant identity check used by non-share workflows).
func PlanShop() common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: shopV202601}
}

// ─────────── v1 path endpoints (spec missing or share parity) ───────────

// PlanUpload describes the push multipart upload for dry-run rendering only.
// Actual multipart upload is performed via client.DoRaw + RawRequest.Headers
// inside push.go's Execute path. Body holds the multipart description map for
// human-readable dry-run output.
func PlanUpload(themeID, name, version string) common.PlannedRequest {
	return common.PlannedRequest{
		Method: "POST",
		Path:   themeBaseV1 + "/upload",
		Query: map[string]any{
			"name":              name,
			"version":           version,
			"merchant_theme_id": "",
			"theme_id":          themeID,
		},
		Body: map[string]any{
			"_kind":         "multipart/form-data",
			"_content_type": "multipart/form-data; boundary=<runtime>",
			"_parts": []map[string]any{
				{"name": "file", "filename": "<theme>.zip", "content_type": "application/zip"},
			},
		},
	}
}

// PlanDownload describes GET /openapi/2020-07/themes/{id}/download (v1 streamed zip).
func PlanDownload(themeID string) common.PlannedRequest {
	return common.PlannedRequest{
		Method: "GET",
		Path:   themeBaseV1 + "/" + themeID + "/download",
	}
}

// ─────────── share-only factories (v1 path even for /shop; byte-parity with v1) ───────────

// PlanShareShop is the share-specific GET /shop. Stays on v1 path for byte-exact
// parity with the v1 CLI's share workflow.
func PlanShareShop() common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: shopV1}
}

// PlanShareUpload is the share-specific multipart upload. Accepts an empty
// themeID (creates a new theme on the share-target shop); otherwise identical
// in shape to PlanUpload. Dry-run only; real upload is via client.DoRaw.
func PlanShareUpload(themeID, name, version string) common.PlannedRequest {
	return common.PlannedRequest{
		Method: "POST",
		Path:   themeBaseV1 + "/upload",
		Query: map[string]any{
			"name":              name,
			"version":           version,
			"merchant_theme_id": "",
			"theme_id":          themeID, // may be ""
		},
		Body: map[string]any{
			"_kind":         "multipart/form-data",
			"_content_type": "multipart/form-data; boundary=<runtime>",
			"_parts": []map[string]any{
				{"name": "file", "filename": "<theme>.zip", "content_type": "application/zip"},
			},
		},
	}
}
