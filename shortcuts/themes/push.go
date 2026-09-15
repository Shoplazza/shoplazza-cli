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

	"github.com/Shoplazza/shoplazza-cli/v2/internal/asynctask"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// pushPollOpts controls the upload-task polling cadence (3s interval, 10-minute cap); tests swap it.
var pushPollOpts = asynctask.PollOptions{
	Interval:    3 * time.Second,
	MaxDuration: 10 * time.Minute,
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
//   - 4xx → deterministic (bad task id, auth, malformed): never recovers → NOT transient,
//     except a non-JSON 404, which is the gateway router rather than the API.
//   - ctx canceled → deliberate stop → NOT transient.
//   - 5xx → server hiccup → transient.
//   - non-HTTP (dial/client timeout/conn reset) → transient connectivity.
func transientPollError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return httpErr.StatusCode >= 500 || isGatewayNotFound(httpErr)
	}
	return true
}

// isGatewayNotFound reports a plain-text 404 from the gateway router; the API's
// own 404 carries a JSON body.
func isGatewayNotFound(e *client.HTTPError) bool {
	return e.StatusCode == http.StatusNotFound && !json.Valid([]byte(e.Body))
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
	Use:     "push --theme-id <id> [--task-id <id>]",
	Short:   "Package cwd, upload to remote theme, and poll the upload task",
	Flags: []common.Flag{
		{
			Name:        "theme-id",
			Short:       "t",
			Type:        common.FlagString,
			Required:    true,
			Description: "Theme ID (required). Run `shoplazza themes list` to discover.",
		},
		{
			Name:        "task-id",
			Type:        common.FlagString,
			Description: "Resume waiting for an earlier upload task instead of uploading again (task_id from a timeout error).",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		themeID, err := theme.RequireThemeID(in.Flags.GetString("theme-id"))
		if err != nil {
			return common.ExecResult{}, err
		}
		taskID := in.Flags.GetString("task-id")
		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}

		// Dry-run: emit the v2 detail, v1 upload, and a placeholder
		// PlanTaskDetail so users see the full request shape. No file I/O,
		// no auth wire-up.
		if in.DryRun {
			if taskID != "" {
				return common.ExecResult{Plans: []common.PlannedRequest{
					PlanDetail(themeID),
					PlanTaskDetail(taskID),
				}}, nil
			}
			// readThemeInfo may fail in dry-run if the cwd isn't a theme;
			// fall back to "<placeholder>" semantics — best-effort
			// name/version with zero-value fallbacks.
			name, version := themeInfoForDryRun(cwd)
			return common.ExecResult{Plans: []common.PlannedRequest{
				PlanDetail(themeID),
				PlanUpload(themeID, name, version),
				PlanTaskDetail("<task_id-from-upload>"),
			}}, nil
		}

		// Step 0: read theme metadata (only a fresh upload needs it).
		var name, version string
		if taskID == "" {
			if name, version, err = readThemeInfo(cwd); err != nil {
				return common.ExecResult{}, err
			}
		}

		// Step 1: detail GET — confirms the theme exists. A 404 here means
		// the upload would also fail and there's no point packaging a zip
		// the user can't deliver.
		detail := PlanDetail(themeID)
		if _, derr := common.Send(ctx, in.Client, detail); derr != nil {
			return common.ExecResult{}, classifyHTTPErr(derr, themeID)
		}

		prog := output.NewProgress(os.Stderr)
		if taskID == "" {
			if taskID, err = packAndUpload(ctx, in.Client, prog, cwd, themeID, name, version); err != nil {
				return common.ExecResult{}, err
			}
			fmt.Fprintf(os.Stderr, "[push] upload task %s\n", taskID)
		} else {
			fmt.Fprintf(os.Stderr, "[push] resuming upload task %s\n", taskID)
		}

		// Step 4: wait for the task to reach a terminal state. The task id is
		// printed on its own line: the spinner label must stay within a terminal row.
		waitStep := prog.Begin("[push] waiting for the server to process the theme")
		payload, err := waitUploadTask(ctx, in.Client, taskID)
		if err != nil {
			waitStep.Fail()
			return common.ExecResult{}, err
		}
		waitStep.Done()
		// The server ships task.info and task.manifest as JSON-encoded STRINGS
		// (e.g. info: "{\"theme_id\":...}"). Decode them into real nested JSON so
		// the result prints cleanly instead of as an escaped \" blob.
		decodeTaskJSONFields(payload)
		return common.ExecResult{Body: map[string]any{
			"theme_id": themeID,
			"task":     payload,
		}}, nil
	},
}

// packAndUpload zips cwd, uploads it to themeID, and returns the upload task id.
func packAndUpload(ctx context.Context, c *client.Client, prog *output.Progress, cwd, themeID, name, version string) (string, error) {
	// Step 2: pack cwd into a tmp zip. Defer cleanup unconditionally
	// — even on later failures the artifact has no diagnostic value
	// (the user can rebuild it from cwd).
	pkgStep := prog.Begin("[push] packaging theme files")
	zipName := themeZipName(name, version)
	zipPath, err := pack.Pack(cwd, zipName, pack.PackOptions{})
	if err != nil {
		pkgStep.Fail()
		return "", theme.ErrLocalIO("pack zip", err)
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
		return "", theme.ErrLocalIO("build multipart", err)
	}
	upload := PlanUpload(themeID, name, version)
	resp, err := c.DoRaw(ctx, client.RawRequest{
		Method:    upload.Method,
		Path:      upload.Path,
		Params:    upload.Query,
		Data:      body,
		Headers:   map[string]string{"Content-Type": ct},
		NoTimeout: true,
	})
	if err != nil {
		uplStep.Fail()
		return "", classifyHTTPErr(err, themeID)
	}
	taskID := extractTaskID(resp.Body)
	if taskID == "" {
		uplStep.Fail()
		return "", theme.ErrLocalIO(
			"upload response missing task_id",
			fmt.Errorf("response body: %v", resp.Body))
	}
	uplStep.Done()
	return taskID, nil
}

// waitUploadTask polls an upload task until it ends and returns its payload.
// Timeouts and task failures come back as envelopes.
func waitUploadTask(ctx context.Context, c *client.Client, taskID string) (map[string]any, error) {
	waitStart := time.Now()
	consecutivePollErrors := 0
	fetch := func(ctx context.Context) (asynctask.Status, error) {
		tr, err := common.Send(ctx, c, PlanTaskDetail(taskID))
		if err != nil {
			// Transient errors are tolerated up to maxConsecutivePollErrors in a row.
			if ctx.Err() != nil || !transientPollError(err) {
				return asynctask.Status{}, err
			}
			consecutivePollErrors++
			if consecutivePollErrors >= maxConsecutivePollErrors {
				return asynctask.Status{}, err
			}
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
		if errors.Is(err, asynctask.ErrTimeout) {
			return st.Payload, theme.ErrTaskTimeout(time.Since(waitStart), pushPollOpts.MaxDuration, taskID, st.Payload)
		}
		if errors.Is(err, context.Canceled) {
			return st.Payload, theme.ErrTaskInterrupted(taskID)
		}
		return st.Payload, err
	}
	if !st.Success {
		return st.Payload, theme.ErrTaskBusinessFailure(st.Payload)
	}
	return st.Payload, nil
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
