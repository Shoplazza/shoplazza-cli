package themecmd

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"strconv"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/asynctask"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
)

// waitUploadTask polls an upload task until it ends and returns its payload.
// Client-based (uses client.DoRaw) so it needs no shortcut engine. Timeouts and
// task failures come back as structured envelopes.
//
// Task status codes (numeric): 0 running, 1 success, 2 failure, other terminal-fail.
func waitUploadTask(ctx context.Context, c *client.Client, taskID string) (map[string]any, error) {
	waitStart := time.Now()
	consecutive := 0
	fetch := func(ctx context.Context) (asynctask.Status, error) {
		resp, err := c.DoRaw(ctx, client.RawRequest{Method: "GET", Path: themeBaseV202601 + "/task/" + taskID})
		if err != nil {
			if ctx.Err() != nil || !transientPollError(err) {
				return asynctask.Status{}, err
			}
			consecutive++
			if consecutive >= maxConsecutivePollErrors {
				return asynctask.Status{}, err
			}
			return asynctask.Status{Done: false}, nil
		}
		consecutive = 0
		task := extractTaskPayload(asMap(resp.Body))
		code := taskStatusCode(task["status"])
		return asynctask.Status{
			Done:    code != 0,
			Success: code == 1,
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

// transientPollError reports whether a task-poll error is worth retrying: 5xx
// and non-HTTP connectivity are transient; a deterministic 4xx (except a
// gateway plain-text 404) or ctx cancel is not.
func transientPollError(err error) bool {
	if errors.Is(err, context.Canceled) {
		return false
	}
	var he *client.HTTPError
	if errors.As(err, &he) {
		return he.StatusCode >= 500 || isGatewayNotFound(he)
	}
	return true
}

func isGatewayNotFound(e *client.HTTPError) bool {
	return e.StatusCode == http.StatusNotFound && !json.Valid([]byte(e.Body))
}

// classifyHTTPErr maps a *client.HTTPError into the right theme envelope; non-HTTP
// errors pass through for the caller's transport classification.
func classifyHTTPErr(err error, themeID string) error {
	var he *client.HTTPError
	if !errors.As(err, &he) {
		return err
	}
	switch he.StatusCode {
	case http.StatusUnauthorized, http.StatusForbidden:
		return theme.ErrAuthExpired(err).WithRequestID(he.RequestID)
	case http.StatusNotFound:
		return theme.ErrValidation("theme not found: %s (run `shoplazza themes list` to see available IDs)", themeID).WithRequestID(he.RequestID)
	case http.StatusBadRequest, http.StatusUnprocessableEntity:
		return theme.ErrValidation("%s", he.Body).WithRequestID(he.RequestID)
	default:
		return err
	}
}

// decodeTaskJSONFields replaces task fields the server ships as JSON-encoded
// strings ("info", "manifest") with their parsed value. Mutates in place.
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

// extractTaskID finds the async task id in an upload response, probing the root
// and a "data" wrap for the flat / single- / double-nested shapes the endpoint
// has shipped. Accepts json.Number or string ids.
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

func idField(m map[string]any, key string) string {
	switch s := m[key].(type) {
	case string:
		return s
	case json.Number:
		return s.String()
	}
	return ""
}

// extractTaskPayload unwraps the task object (carrying status/info) from a poll
// response, handling the unwrapped ({task:{...}}) and wrapped ({data:{task:{...}}}
// / {data:{...}}) shapes.
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

// taskStatusCode coerces a status field into a numeric code (json.Number /
// float64 / int / numeric string); anything else is 0 ("still running").
func taskStatusCode(v any) float64 {
	switch s := v.(type) {
	case float64:
		return s
	case int:
		return float64(s)
	case json.Number:
		if fl, err := s.Float64(); err == nil {
			return fl
		}
	case string:
		if fl, err := strconv.ParseFloat(s, 64); err == nil {
			return fl
		}
	}
	return 0
}

func getString(m map[string]any, k string) string {
	if v, ok := m[k].(string); ok {
		return v
	}
	return ""
}

// asMap coerces a response body to a map (empty map when it is not one).
func asMap(v any) map[string]any {
	if m, ok := v.(map[string]any); ok {
		return m
	}
	return map[string]any{}
}

// asString coerces a JSON scalar (string / json.Number / bool) to a string.
func asString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case json.Number:
		return t.String()
	case bool:
		return strconv.FormatBool(t)
	default:
		return ""
	}
}
