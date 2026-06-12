package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"os"
	"strconv"
	"time"

	"shoplazza-cli-v2/internal/asynctask"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/multipartx"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/theme"
	"shoplazza-cli-v2/internal/theme/pack"
	"shoplazza-cli-v2/shortcuts/common"
)

// pushPollOpts controls the upload-task polling cadence. Declared at
// package scope so tests can swap in tiny intervals via withPushPollOpts;
// production defaults match the v1 CLI (3-second cadence, 3-minute cap).
var pushPollOpts = asynctask.PollOptions{
	Interval:    3 * time.Second,
	MaxDuration: 3 * time.Minute,
}

// maxConsecutivePollErrors bounds how many CONSECUTIVE transient errors (5xx /
// network blips) the task poll tolerates before giving up. The counter resets on
// any successful poll, so scattered hiccups over a long processing window never
// accumulate — only a sustained run of failures (the endpoint genuinely down)
// aborts. At pushPollOpts.Interval cadence this also makes a real outage fail
// fast (≈ budget × Interval) instead of waiting out MaxDuration. Package-scoped
// so tests can shrink it.
var maxConsecutivePollErrors = 5

// transientPollError reports whether a task-poll error is worth retrying rather
// than failing the whole push. The task itself keeps processing server-side even
// when the status endpoint blips, and a task *failure* comes back as HTTP 200
// with status=2 (not a 5xx), so retrying a read error never masks a real failure.
//
//   - 4xx → deterministic (bad task id, auth, malformed): never recovers → NOT transient.
//   - ctx canceled/deadline → deliberate stop → NOT transient.
//   - 5xx → server hiccup → transient.
//   - non-HTTP (dial/timeout/conn reset) → transient connectivity.
func transientPollError(err error) bool {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return false
	}
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500
	}
	return true
}

// pushShortcut is the `themes push` workflow: zip the cwd, multipart-upload it
// to the v1 upload endpoint (no v2 equivalent yet), then poll the v2 task
// endpoint until the server reports terminal state. The upload step uses
// client.DoRaw rather than common.Send because PlannedRequest carries no
// Headers field for the multipart Content-Type.
//
// Polling status codes (numeric, from the task API):
//
//	0 → still running    (continue polling)
//	1 → success          (terminate, return OK)
//	2 → failure          (terminate, ErrTaskBusinessFailure)
//	other → treated as terminal-non-success (ErrTaskBusinessFailure)
var pushShortcut = common.Shortcut{
	Service: "themes",
	Command: "push",
	Use:     "push --theme-id <id>",
	Short:   "Package cwd, upload to remote theme, and poll the upload task",
	Flags: []common.Flag{
		{
			Name:        "theme-id",
			Short:       "t",
			Type:        common.FlagString,
			Required:    true,
			Description: "Theme ID (required). Run `shoplazza themes list` to discover.",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		themeID, err := theme.RequireThemeID(in.Flags.GetString("theme-id"))
		if err != nil {
			return common.ExecResult{}, err
		}
		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}

		// Dry-run: emit the v2 detail, v1 upload, and a placeholder
		// PlanTaskDetail so users see the full request shape. No file I/O,
		// no auth wire-up.
		if in.DryRun {
			// readThemeInfo may fail in dry-run if the cwd isn't a theme;
			// fall back to "<placeholder>" semantics — best-effort
			// name/version with zero-value fallbacks.
			name, version, _ := readThemeInfo(cwd)
			if name == "" {
				name = "<theme>"
			}
			if version == "" {
				version = "<version>"
			}
			return common.ExecResult{Plans: []common.PlannedRequest{
				PlanDetail(themeID),
				PlanUpload(themeID, name, version),
				PlanTaskDetail("<task_id-from-upload>"),
			}}, nil
		}

		// Step 0: read theme metadata (required live).
		name, version, err := readThemeInfo(cwd)
		if err != nil {
			return common.ExecResult{}, err
		}

		// Step 1: detail GET — confirms the theme exists. A 404 here means
		// the upload would also fail and there's no point packaging a zip
		// the user can't deliver.
		detail := PlanDetail(themeID)
		if _, derr := common.Send(ctx, in.Client, detail); derr != nil {
			return common.ExecResult{}, classifyHTTPErr(derr, themeID)
		}

		// Step 2: pack cwd into a tmp zip. Defer cleanup unconditionally
		// — even on later failures the artifact has no diagnostic value
		// (the user can rebuild it from cwd).
		prog := output.NewProgress(os.Stderr)
		pkgStep := prog.Begin("[push] packaging theme files")
		zipName := themeZipName(name, version)
		zipPath, err := pack.Pack(cwd, zipName, pack.PackOptions{})
		if err != nil {
			pkgStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO("pack zip", err)
		}
		defer os.Remove(zipPath)
		// Size label is best-effort: on stat failure omit it entirely
		// rather than printing a wrong "(0 bytes)".
		sizeLabel := ""
		if zipInfo, statErr := os.Stat(zipPath); statErr == nil {
			sizeLabel = fmt.Sprintf(" (%d bytes)", zipInfo.Size())
		}
		pkgStep.Done()

		// Step 3: multipart upload via client.DoRaw (PlannedRequest has no
		// Headers field for the per-request Content-Type). NoTimeout: a large
		// zip on a slow uplink can exceed the client-wide 30s timeout; ctx
		// still aborts on signal.
		uplStep := prog.Begin(fmt.Sprintf("[push] uploading %s%s", zipName, sizeLabel))
		body, ct, err := multipartx.FileFormBody("file", zipPath, "application/zip", nil)
		if err != nil {
			uplStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO("build multipart", err)
		}
		upload := PlanUpload(themeID, name, version)
		resp, err := in.Client.DoRaw(ctx, client.RawRequest{
			Method:    upload.Method,
			Path:      upload.Path,
			Params:    upload.Query,
			Data:      body,
			Headers:   map[string]string{"Content-Type": ct},
			NoTimeout: true,
		})
		if err != nil {
			uplStep.Fail()
			return common.ExecResult{}, classifyHTTPErr(err, themeID)
		}
		taskID := extractTaskID(resp.Body)
		if taskID == "" {
			uplStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO(
				"upload response missing task_id",
				fmt.Errorf("response body: %v", resp.Body))
		}
		uplStep.Done()

		// Step 4: poll the task until the server reports terminal state. The
		// task API is hit every pushPollOpts.Interval; waitStart feeds the
		// timeout envelope's elapsed_seconds. fetch maps numeric status codes
		// to asynctask.Status, accepting json.Number (UseNumber) or float64.
		waitStart := time.Now()
		waitStep := prog.Begin("[push] waiting for the server to process the theme")
		consecutivePollErrors := 0
		fetch := func(ctx context.Context) (asynctask.Status, error) {
			tr, err := common.Send(ctx, in.Client, PlanTaskDetail(taskID))
			if err != nil {
				// A real error (4xx, ctx cancel) aborts immediately. A transient one
				// (5xx / network blip) is tolerated while the task keeps processing,
				// but only up to maxConsecutivePollErrors in a row, so a genuinely
				// down endpoint still fails fast. Any successful poll resets the
				// streak, so isolated blips never accumulate.
				if !transientPollError(err) {
					return asynctask.Status{}, err
				}
				consecutivePollErrors++
				if consecutivePollErrors >= maxConsecutivePollErrors {
					return asynctask.Status{}, err
				}
				// Not done, no error: Poll sleeps Interval and retries.
				return asynctask.Status{Done: false}, nil
			}
			consecutivePollErrors = 0
			task := extractTaskPayload(tr)
			statusCode := taskStatusCode(task["status"])
			return asynctask.Status{
				Done:    statusCode != 0,
				Success: statusCode == 1,
				Message: getString(task, "message"),
				Payload: task,
			}, nil
		}
		st, err := asynctask.Poll(ctx, fetch, pushPollOpts)
		if err != nil {
			waitStep.Fail()
			if errors.Is(err, asynctask.ErrTimeout) {
				return common.ExecResult{}, theme.ErrTaskTimeout(
					time.Since(waitStart), pushPollOpts.MaxDuration, st.Payload)
			}
			return common.ExecResult{}, err
		}
		if !st.Success {
			waitStep.Fail()
			return common.ExecResult{}, theme.ErrTaskBusinessFailure(st.Payload)
		}
		waitStep.Done()
		// The server ships task.info and task.manifest as JSON-encoded STRINGS
		// (e.g. info: "{\"theme_id\":...}"). Decode them into real nested JSON so
		// the result prints cleanly instead of as an escaped \" blob.
		decodeTaskJSONFields(st.Payload)
		return common.ExecResult{Body: map[string]any{
			"theme_id": themeID,
			"task":     st.Payload,
		}}, nil
	},
}

// decodeTaskJSONFields replaces task fields the server ships as JSON-encoded
// strings ("info", "manifest") with their parsed value, so the CLI renders
// them as real nested JSON instead of an escaped blob. A field that is
// absent, not a string, or does not parse into a JSON object/array is left
// untouched, so scalar strings are never coerced. Mutates the map in place.
func decodeTaskJSONFields(task map[string]any) {
	for _, key := range []string{"info", "manifest"} {
		s, ok := task[key].(string)
		if !ok || s == "" {
			continue
		}
		var decoded any
		if err := json.Unmarshal([]byte(s), &decoded); err != nil {
			continue
		}
		switch decoded.(type) {
		case map[string]any, []any:
			task[key] = decoded
		}
	}
}

// extractTaskID finds the async task id in an upload response. The endpoint
// has shipped several shapes; we probe the root and a "data" wrap for each:
//
//	{"task_id": "..."}                   (flat — test mocks / legacy)
//	{"task": {"task": {"id": "..."}}}    (real upload — double-nested)
//	{"task": {"id": "..."}}              (single-nested, defensive)
//
// Accepts json.Number (UseNumber path) or string ids.
func extractTaskID(body any) string {
	roots := []map[string]any{}
	if m, ok := body.(map[string]any); ok {
		roots = append(roots, m)
		if d, ok := m["data"].(map[string]any); ok {
			roots = append(roots, d)
		}
	}
	for _, r := range roots {
		if id := idField(r, "task_id"); id != "" {
			return id
		}
		if t, ok := r["task"].(map[string]any); ok {
			if id := idField(t, "id"); id != "" {
				return id
			}
			if id := idField(t, "task_id"); id != "" {
				return id
			}
			if tt, ok := t["task"].(map[string]any); ok {
				if id := idField(tt, "id"); id != "" {
					return id
				}
			}
		}
	}
	return ""
}

// idField reads m[key] as a string id, accepting string or json.Number.
func idField(m map[string]any, key string) string {
	switch s := m[key].(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

// extractTaskPayload unwraps the task object (the map carrying status/info)
// from a poll response. The client strips the {data,ok} envelope only on
// ok:true / code:Success responses, so we must handle BOTH the unwrapped and
// wrapped shapes the server uses:
//
//	{"task":{...}}             (unwrapped — real ok:true responses)
//	{"data":{"task":{...}}}    (wrapped — responses without ok)
//	{"data":{...}}             (older; task fields at "data" root)
//
// Prefers a child that looks like a task (has "status"/"info"); otherwise
// falls back to the data-wrap, then the raw response.
func extractTaskPayload(resp map[string]any) map[string]any {
	dataMap, _ := resp["data"].(map[string]any)

	candidates := []map[string]any{}
	if dataMap != nil {
		if t, ok := dataMap["task"].(map[string]any); ok {
			candidates = append(candidates, t)
		}
	}
	if t, ok := resp["task"].(map[string]any); ok {
		candidates = append(candidates, t)
	}
	for _, c := range candidates {
		if isTaskShaped(c) {
			return c
		}
	}
	if len(candidates) > 0 {
		return candidates[0]
	}
	if dataMap != nil {
		return dataMap
	}
	return resp
}

// isTaskShaped reports whether m carries the fields a task poll response
// exposes (status or info).
func isTaskShaped(m map[string]any) bool {
	if m == nil {
		return false
	}
	if _, ok := m["status"]; ok {
		return true
	}
	_, ok := m["info"]
	return ok
}

// taskStatusCode coerces a status field into a float64 status code.
// The client uses json.Decoder.UseNumber, so numeric fields arrive as
// json.Number — but legacy tests / hand-built maps may also pass float64
// or int. Anything else (string, nil) yields 0 ("still running"), which
// is the safe default: the next poll round will resolve it.
func taskStatusCode(v any) float64 {
	switch s := v.(type) {
	case float64:
		return s
	case int:
		return float64(s)
	case json.Number:
		if f, err := s.Float64(); err == nil {
			return f
		}
	case string:
		// The server has returned status as a string ("0"/"1"/"2") on some
		// endpoints; parse defensively so polling still terminates.
		if f, err := strconv.ParseFloat(s, 64); err == nil {
			return f
		}
	}
	return 0
}

// getString returns m[k] as string, or "" if absent or wrong type.
func getString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// classifyHTTPErr maps a *client.HTTPError into the right v2 envelope.
// Non-HTTP errors (dial / timeout / TLS) fall through untouched — the
// engine's classifyExecError will lift them to network-class for us.
func classifyHTTPErr(err error, themeID string) error {
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	switch {
	case httpErr.StatusCode == http.StatusUnauthorized,
		httpErr.StatusCode == http.StatusForbidden:
		return theme.ErrAuthExpired(err)
	case httpErr.StatusCode == http.StatusNotFound:
		return theme.ErrValidation(
			"theme not found: %s (run `shoplazza themes list` to see available IDs)", themeID)
	case httpErr.StatusCode == http.StatusBadRequest,
		httpErr.StatusCode == http.StatusUnprocessableEntity:
		return theme.ErrValidation("server rejected request: %s", httpErr.Body)
	default:
		return err
	}
}
