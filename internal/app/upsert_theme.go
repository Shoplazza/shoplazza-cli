package app

import (
	"context"
	"encoding/json"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// UpsertResult is the outcome of an extension-upsert leg (theme today; reused by
// the function leg later). Mirrors v1 upsertTheme's resolved value
// {extension_id, extension_version, extension_version_id}.
type UpsertResult struct {
	ExtensionID        string
	ExtensionVersion   string
	ExtensionVersionID string
}

// store-openapi (2020-07) flat response bodies. These endpoints are NOT
// enveloped — v1 reads the fields directly off response.data, and the v2 client's
// unmarshalUnwrapped leaves a body with no {data} key untouched, so a typed
// struct decodes the flat body directly.
type themeUpsertResp struct {
	ExtensionID string `json:"extension_id"`
}

type themeVersionTaskCreateResp struct {
	TaskID string `json:"task_id"`
}

type themeVersionTaskResp struct {
	TaskID    string `json:"task_id"`
	State     int    `json:"state"` // 1 = success, 2 = failed, else still running
	Message   string `json:"message"`
	VersionID string `json:"version_id"`
}

// partner-openapi (2025-06) connection response. The SAME endpoint
// has been observed with two success markers — v1 te judged code===200 (int),
// v1 app-deploy's connectTheme judged code=="Success" (string). Decode code as
// RawMessage and accept BOTH so app deploy and te connect share one judgment.
type themeConnectionResp struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

// ConnectionSucceeded reports success for either historical marker. Exported
// so the te package reuses the SAME judgment for the publications endpoint
// (its 2xx bodies carry the same {code,message} envelope).
func ConnectionSucceeded(raw json.RawMessage) bool {
	var v any
	if err := json.Unmarshal(raw, &v); err != nil {
		return false
	}
	switch t := v.(type) {
	case float64: // JSON numbers decode to float64
		return t == 200
	case string:
		return strings.EqualFold(t, "success") || t == "200"
	}
	return false
}

// RegisterThemeExtension performs the no-connect registration segment shared by
// app deploy (theme leg) and te build/serve: PUT theme-extensions (create when
// ext.ExtensionID empty, else update with the existing id), POST version-tasks,
// poll to completion. It NEVER calls the connection endpoint — connect is an
// explicit step (deploy's create path, or `te connect`) so te can register
// without binding the extension to an app.
//
// Version: an explicit ext.ExtensionVersion always wins (update bump OR a chosen
// create version, e.g. te build --version on a fresh extension); falls back to
// "1.0.0" only when none was supplied (app deploy's create path always sets
// "1.0.0", so its behavior is unchanged).
func RegisterThemeExtension(ctx context.Context, ext Extension, store *client.Client, pollInterval time.Duration, maxRetry int) (UpsertResult, *output.ExitError) {
	update := ext.ExtensionID != "" && ext.ExtensionVersion != ""
	version := "1.0.0"
	if ext.ExtensionVersion != "" {
		version = ext.ExtensionVersion
	}

	putBody := map[string]any{
		"title":        ext.ExtensionName,
		"resource_url": ext.ResourceURL,
	}
	if update {
		putBody["extension_id"] = ext.ExtensionID
	}
	var put themeUpsertResp
	if err := store.PutJSON(ctx, "/openapi/2020-07/theme-extensions", putBody, &put); err != nil {
		return UpsertResult{}, apiOrInternal(err)
	}
	extensionID := put.ExtensionID
	if extensionID == "" {
		extensionID = ext.ExtensionID
	}
	if extensionID == "" {
		return UpsertResult{}, output.ErrInternal("theme extension %q upsert returned no extension_id", ext.ExtensionName)
	}

	var task themeVersionTaskCreateResp
	taskBody := map[string]any{
		"extension_id": extensionID,
		"version":      version,
		"resource_url": ext.ResourceURL,
	}
	if ext.Exts != "" {
		taskBody["exts"] = ext.Exts // version description (te build --version); v1 field name
	}
	if err := store.PostJSON(ctx, "/openapi/2020-07/theme-extensions/version-tasks", taskBody, &task); err != nil {
		return UpsertResult{}, apiOrInternal(err)
	}

	versionID, perr := PollThemeVersionTask(ctx, store, ext.ExtensionName, task.TaskID, pollInterval, maxRetry)
	if perr != nil {
		return UpsertResult{}, perr
	}
	return UpsertResult{ExtensionID: extensionID, ExtensionVersion: version, ExtensionVersionID: versionID}, nil
}

// upsertTheme = RegisterThemeExtension + (create path only) ConnectTheme. Used by
// app deploy; preserves v1 behavior (update path does NOT connect).
func upsertTheme(ctx context.Context, ext Extension, store, partner *client.Client, pollInterval time.Duration, maxRetry int) (UpsertResult, *output.ExitError) {
	update := ext.ExtensionID != "" && ext.ExtensionVersion != ""
	r, err := RegisterThemeExtension(ctx, ext, store, pollInterval, maxRetry)
	if err != nil {
		return UpsertResult{}, err
	}
	if update {
		return r, nil
	}
	if cerr := ConnectTheme(ctx, partner, ext.ExtensionName, r.ExtensionID, ThemeConnectionPathApp); cerr != nil {
		return UpsertResult{}, cerr
	}
	return r, nil
}

// PollThemeVersionTask polls GET version-tasks/{taskID} up to maxRetry times,
// waiting pollInterval between polls. state 1 → success (version_id); state 2 →
// failure (message); exhausting the retries → timeout. Polls before sleeping so
// an already-finished/failed task is detected on the first iteration.
//
// extRef is a display-only reference to the extension — app deploy passes the
// extension NAME, te's dev push passes the extension ID (the name isn't in
// scope there); the message is honest for both.
//
// A state-2 task and a poll timeout are SERVER-reported outcomes (the backend
// accepted the task and then failed / never finished it), not CLI bugs — both
// are API-class, not internal.
func PollThemeVersionTask(ctx context.Context, store *client.Client, extRef, taskID string, pollInterval time.Duration, maxRetry int) (string, *output.ExitError) {
	path := "/openapi/2020-07/theme-extensions/version-tasks/" + taskID
	for i := 0; i < maxRetry; i++ {
		var res themeVersionTaskResp
		if err := store.GetJSON(ctx, path, &res); err != nil {
			return "", apiOrInternal(err)
		}
		switch res.State {
		case 1:
			return res.VersionID, nil
		case 2:
			msg := res.Message
			if msg == "" {
				msg = "create theme extension version failed"
			}
			return "", output.Errorf(output.ExitAPI, output.TypeAPI,
				"theme extension %q version task failed: %s", extRef, msg)
		}
		select {
		case <-ctx.Done():
			return "", output.ErrInternal("theme extension %q version task cancelled: %v", extRef, ctx.Err())
		case <-time.After(pollInterval):
		}
	}
	return "", output.Errorf(output.ExitAPI, output.TypeAPI,
		"theme extension %q version task timed out after %d polls", extRef, maxRetry)
}

// Theme-extension connection endpoints — two flows, each matching its v1 source:
//   - standalone `te connect` + `te release`: 2024-07 for BOTH connection and
//     publications (v1 lib/partner-api/axios.js pinned /openapi/2024-07). 2025-06
//     has no /publications at all.
//   - `app deploy`'s theme leg: 2025-06 connection (v1 lib/app/api/partnerOpenapi.js
//     posts openapi/2025-06/theme-extensions/connection). Its publish is a separate
//     STORE-side 2020-07/publications call, so it never touches the 2024-07
//     publications endpoint and the connect/publish version pairing is irrelevant.
//
// NOTE: "version has no doc" from the publish endpoint is NOT a connection-version
// problem. It means the built version has no parsed doctree, caused by a malformed
// upload bundle — see ZipThemeApp (the "theme-app/" wrapper must be present) and the
// te templates (assets-manifest.json must sit at the theme-app root).
const (
	ThemeConnectionPathApp        = "/openapi/2025-06/theme-extensions/connection"
	ThemeConnectionPathStandalone = "/openapi/2024-07/theme-extensions/connection"
)

// ConnectTheme POSTs the connection (type:"link") via the partner client at
// connPath. Exported so te connect reuses the SAME body + judgment.
func ConnectTheme(ctx context.Context, partner *client.Client, extName, extensionID, connPath string) *output.ExitError {
	var res themeConnectionResp
	body := map[string]any{
		"extension_id": extensionID,
		"type":         "link",
	}
	if err := partner.PostJSON(ctx, connPath, body, &res); err != nil {
		return apiOrInternal(err)
	}
	// The live success shape is 200 + {"code":"Success","data":{}} — but the
	// generic client STRIPS that envelope before we decode, leaving an empty
	// code. So a 2xx is success UNLESS it carries an EXPLICIT non-success code:
	//   - empty/absent code  → the {"code":"Success",data} envelope was unwrapped → success
	//   - int 200 / "Success" that survived unstripped (no data key) → ConnectionSucceeded → success
	//   - any other present code (e.g. 4001) → real failure (v1 parity: code must be Success/200)
	if len(res.Code) > 0 && !ConnectionSucceeded(res.Code) {
		msg := res.Message
		if msg == "" {
			msg = "connection theme extension failed"
		}
		// A non-success business code in a 2xx body is the SERVER rejecting the
		// connection — API-class, not a CLI bug.
		return output.Errorf(output.ExitAPI, output.TypeAPI,
			"theme extension %q connection failed: %s", extName, msg)
	}
	return nil
}
