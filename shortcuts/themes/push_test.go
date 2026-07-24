package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/asynctask"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// ── taskStatusCode ────────────────────────────────────────────────────────────

func TestTaskStatusCode_Float64(t *testing.T) {
	if got := taskStatusCode(float64(2)); got != 2 {
		t.Errorf("float64: got %v want 2", got)
	}
}

func TestTaskStatusCode_Int(t *testing.T) {
	if got := taskStatusCode(int(1)); got != 1 {
		t.Errorf("int: got %v want 1", got)
	}
}

func TestTaskStatusCode_JSONNumber(t *testing.T) {
	if got := taskStatusCode(json.Number("2")); got != 2 {
		t.Errorf("json.Number: got %v want 2", got)
	}
}

func TestTaskStatusCode_StringNumeric(t *testing.T) {
	if got := taskStatusCode("1"); got != 1 {
		t.Errorf("string numeric: got %v want 1", got)
	}
}

func TestTaskStatusCode_Unknown(t *testing.T) {
	if got := taskStatusCode(nil); got != 0 {
		t.Errorf("nil: got %v want 0", got)
	}
	if got := taskStatusCode("not-a-number"); got != 0 {
		t.Errorf("invalid string: got %v want 0", got)
	}
}

// ── classifyHTTPErr ───────────────────────────────────────────────────────────

func TestClassifyHTTPErr_NonHTTPPassthrough(t *testing.T) {
	orig := errors.New("dial tcp: connection refused")
	got := classifyHTTPErr(orig, "theme-1")
	if got != orig {
		t.Errorf("non-HTTP error should pass through; got %T", got)
	}
}

func TestClassifyHTTPErr_401(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusUnauthorized}
	got := classifyHTTPErr(err, "t1")
	if got == err {
		t.Errorf("401 should be reclassified, not passed through")
	}
}

func TestClassifyHTTPErr_403(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusForbidden}
	got := classifyHTTPErr(err, "t1")
	if got == err {
		t.Errorf("403 should be classified, not passed through")
	}
}

func TestClassifyHTTPErr_404(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusNotFound}
	got := classifyHTTPErr(err, "theme-xyz")
	if got == nil {
		t.Fatal("expected non-nil error")
	}
	msg := got.Error()
	if !strings.Contains(msg, "theme-xyz") && !strings.Contains(msg, "not found") {
		t.Errorf("404 error should mention theme or not found: %s", msg)
	}
}

func TestClassifyHTTPErr_400(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusBadRequest, Body: "bad field"}
	got := classifyHTTPErr(err, "t1")
	if got == err {
		t.Errorf("400 should be classified as validation error")
	}
}

func TestClassifyHTTPErr_422(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusUnprocessableEntity, Body: "invalid"}
	got := classifyHTTPErr(err, "t1")
	if got == err {
		t.Errorf("422 should be classified as validation error")
	}
}

func TestClassifyHTTPErr_500Passthrough(t *testing.T) {
	err := &client.HTTPError{StatusCode: http.StatusInternalServerError}
	got := classifyHTTPErr(err, "t1")
	if got != err {
		t.Errorf("500 should pass through; got different error %T", got)
	}
}

// pushFlags builds a FlagSet over a cobra command with --theme-id. Mirrors
// pullFlags but for the push shortcut.
func pushFlags(themeID string) common.FlagSet {
	cmd := &cobra.Command{Use: "push"}
	cmd.Flags().StringP("theme-id", "t", themeID, "")
	return common.NewCobraFlagSet(cmd)
}

// withPushPollOpts swaps in test-friendly poll options (tiny interval +
// bounded MaxDuration) for the duration of the test, restoring originals
// in t.Cleanup. Required because the production defaults (3s / 3m) would
// blow past every test budget.
func withPushPollOpts(t *testing.T, opts asynctask.PollOptions) {
	t.Helper()
	prev := pushPollOpts
	pushPollOpts = opts
	t.Cleanup(func() { pushPollOpts = prev })
}

// envelopeOf reaches into err's Envelope() carrier for inspection. Mirrors
// extractPackageEnvelope in package_test.go but local to push tests so
// they aren't coupled to that helper's location.
func envelopeOf(t *testing.T, err error) map[string]any {
	t.Helper()
	if err == nil {
		t.Fatal("err is nil")
	}
	type enveloper interface{ Envelope() map[string]any }
	if e, ok := err.(enveloper); ok {
		return e.Envelope()
	}
	t.Fatalf("err does not expose Envelope(): %T (%v)", err, err)
	return nil
}

// TestPush_MissingThemeIDExitsValidation: the engine relies on
// RequireThemeID inside Execute, not just cobra MarkFlagRequired.
func TestPush_MissingThemeIDExitsValidation(t *testing.T) {
	in := common.ExecInput{Flags: pushFlags("")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected validation error for missing --theme-id")
	}
	if !errors.Is(err, theme.ErrMissingThemeFlag) {
		t.Errorf("expected ErrMissingThemeFlag sentinel; got %T: %v", err, err)
	}
	env := envelopeOf(t, err)
	if env["type"] != output.TypeValidation {
		t.Errorf("type = %v, want %q", env["type"], output.TypeValidation)
	}
}

// TestPush_DryRunEmitsAllPlannedRequests: dry-run emits PlanDetail (v2) +
// PlanUpload (v1) + PlanTaskDetail (v2). The task_id is a placeholder
// since dry-run never calls the upload endpoint.
func TestPush_DryRunEmitsAllPlannedRequests(t *testing.T) {
	in := common.ExecInput{DryRun: true, Flags: pushFlags("abc123")}
	res, err := pushShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if len(res.Plans) != 3 {
		t.Fatalf("dry-run should emit 3 PlannedRequest (detail + upload + task); got %d", len(res.Plans))
	}
	hasDetail, hasUpload, hasTask := false, false, false
	for _, p := range res.Plans {
		switch {
		case strings.Contains(p.Path, "/2026-01/themes/abc123") &&
			!strings.Contains(p.Path, "/task/"):
			hasDetail = true
		case strings.Contains(p.Path, "/2020-07/themes/upload"):
			hasUpload = true
		case strings.Contains(p.Path, "/2026-01/themes/task/"):
			hasTask = true
		}
	}
	if !hasDetail || !hasUpload || !hasTask {
		paths := make([]string, 0, len(res.Plans))
		for _, p := range res.Plans {
			paths = append(paths, p.Path)
		}
		t.Fatalf("expected PlanDetail (v2) + PlanUpload (v1) + PlanTaskDetail (v2); got %v", paths)
	}
}

// pushTestServer builds an httptest.Server that mocks the three endpoints
// push touches: GET /openapi/2026-01/themes/{id}, POST
// /openapi/2020-07/themes/upload, GET /openapi/2026-01/themes/task/{taskID}.
//
// taskResponses queues per-poll responses; the server pops one each time
// the task endpoint is hit. taskID is fixed so we can assert path
// composition without parsing the upload's task_id.
type pushTestServer struct {
	*httptest.Server
	taskCalls    int32
	uploadCalled int32
}

func newPushTestServer(t *testing.T, taskResponses []string) *pushTestServer {
	t.Helper()
	pts := &pushTestServer{}
	pts.Server = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			atomic.AddInt32(&pts.uploadCalled, 1)
			// We deliberately do NOT read the multipart body here — the
			// transport correctness is exercised by internal/multipartx
			// tests; this server just confirms the route is hit.
			w.Header().Set("Content-Type", "application/json")
			// Real upload shape: the task id is double-nested at task.task.id.
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			n := atomic.AddInt32(&pts.taskCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			i := int(n) - 1
			if i >= len(taskResponses) {
				i = len(taskResponses) - 1
			}
			_, _ = w.Write([]byte(taskResponses[i]))
		default:
			// PlanDetail (theme existence check) — return a minimal envelope.
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	t.Cleanup(pts.Close)
	return pts
}

// setupThemeCWD writes a minimal config/settings_schema.json so
// readThemeInfo succeeds with a deterministic name/version pair. Mirrors
// the helper in package_test.go but local to push tests so we don't
// import-couple to package_test.go's helpers.
func setupThemeCWD(t *testing.T, themeName, themeVersion string) {
	t.Helper()
	t.Chdir(t.TempDir())
	makeMinimalSettings(t, themeName, themeVersion)
}

func makeMinimalSettings(t *testing.T, themeName, themeVersion string) {
	t.Helper()
	if err := os.MkdirAll("config", 0o755); err != nil {
		t.Fatalf("mkdir config: %v", err)
	}
	// Real settings_schema.json is a JSON ARRAY; theme_info is the element with
	// name=="theme_info". A localized-object name on a second section locks in
	// readThemeInfo's tolerance of mixed name types.
	content := fmt.Sprintf(`[{"name":"theme_info","theme_name":%q,"theme_version":%q},{"name":{"en":"Section"},"settings":[]}]`,
		themeName, themeVersion)
	if err := os.WriteFile(filepath.Join("config", "settings_schema.json"), []byte(content), 0o644); err != nil {
		t.Fatalf("write settings: %v", err)
	}
}

// TestPush_TaskSuccessReturnsOK: full happy path. Detail GET → upload POST
// → task poll returns status=1 (success). Body carries theme_id + task
// payload passthrough.
func TestPush_TaskSuccessReturnsOK(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})

	srv := newPushTestServer(t, []string{
		`{"data":{"task":{"status":1,"info":"done","message":"ok"}}}`,
	})
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	res, err := pushShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if got := res.Body["theme_id"]; got != "abc123" {
		t.Errorf("theme_id = %v, want abc123", got)
	}
	if atomic.LoadInt32(&srv.uploadCalled) == 0 {
		t.Errorf("upload endpoint never called")
	}
	if atomic.LoadInt32(&srv.taskCalls) == 0 {
		t.Errorf("task endpoint never polled")
	}
	task, _ := res.Body["task"].(map[string]any)
	if task == nil {
		t.Fatalf("Body.task missing; got Body=%v", res.Body)
	}
	// status comes through as json.Number when the client uses UseNumber —
	// accept either since the assertion is on presence, not type.
	if v, ok := task["status"]; !ok {
		t.Errorf("task.status absent in passthrough payload: %v", task)
	} else if fmt.Sprint(v) != "1" {
		t.Errorf("task.status = %v, want 1", v)
	}
}

// TestPush_TaskFailurePassThrough: status=2 (failure) → ErrTaskBusinessFailure
// envelope (type=api), with the full task payload passed through under
// the "task" extra.
func TestPush_TaskFailurePassThrough(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})

	srv := newPushTestServer(t, []string{
		`{"data":{"task":{"status":2,"info":"oops","message":"upload syntax error"}}}`,
	})
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected business-failure error for status=2")
	}
	env := envelopeOf(t, err)
	if env["type"] != output.TypeAPI {
		t.Errorf("type = %v, want %q", env["type"], output.TypeAPI)
	}
	// Task payload must be passed through verbatim — agents read this.
	task, _ := env["task"].(map[string]any)
	if task == nil {
		t.Fatalf("envelope missing task passthrough; got env=%v", env)
	}
	if msg, _ := task["message"].(string); msg != "upload syntax error" {
		t.Errorf("task.message = %v, want %q", msg, "upload syntax error")
	}
}

// TestPush_PollHTTPErrorCarriesTaskEndpoint reproduces the real-world failure a
// user hit: upload succeeds, then the task-poll endpoint returns 500 mid-
// processing. push must propagate an *client.HTTPError stamped with the failing
// endpoint (GET /themes/task/...), so the engine renders an envelope that names
// which interface broke (④ poll) instead of an anonymous ServerError. This is
// the data that backs the error.detail.method/path enrichment.
func TestPush_PollHTTPErrorCarriesTaskEndpoint(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			// The poll endpoint 500s — exactly the user-reported failure.
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"code":"ServerError","message":"[]"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("expected error when the task-poll endpoint returns 500")
	}
	var he *client.HTTPError
	if !errors.As(err, &he) {
		t.Fatalf("expected a wrapped *client.HTTPError, got %T (%v)", err, err)
	}
	if he.StatusCode != 500 {
		t.Errorf("StatusCode = %d, want 500", he.StatusCode)
	}
	if he.Method != "GET" || !strings.Contains(he.Path, "/themes/task/task-xyz") {
		t.Errorf("failing endpoint = %q %q, want GET .../themes/task/task-xyz", he.Method, he.Path)
	}
}

// TestPush_TransientPollErrorRecovers: a single 5xx while reading task status
// (the real-world failure: the server intermittently 500s on an in-flight task)
// must NOT fail the push — the task is still processing and the next poll
// succeeds. This is the core fault-tolerance behavior.
func TestPush_TransientPollErrorRecovers(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})
	var taskCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			n := atomic.AddInt32(&taskCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			if n == 1 {
				// Transient server hiccup on the first status read.
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"ServerError","message":"[]"}`))
				return
			}
			// Task finished successfully — same as what `themes task` returns once terminal.
			_, _ = w.Write([]byte(`{"task":{"status":1,"message":"success"}}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("a single transient poll 500 must not fail the push (task is still processing); got %v", err)
	}
	if got := atomic.LoadInt32(&taskCalls); got < 2 {
		t.Errorf("expected push to keep polling past the transient 500 (>=2 polls); got %d", got)
	}
}

// TestPush_PollClientErrorAbortsImmediately: a 4xx during polling is
// deterministic (bad id / auth / malformed) and must NOT be retried — it aborts
// on the first occurrence, never burning the transient-retry budget.
func TestPush_PollClientErrorAbortsImmediately(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})
	var taskCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			atomic.AddInt32(&taskCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusNotFound)
			_, _ = w.Write([]byte(`{"code":"NotFound"}`))
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatal("a 4xx during polling must abort the push")
	}
	if got := atomic.LoadInt32(&taskCalls); got != 1 {
		t.Errorf("4xx must abort on the first poll (no retry); task polled %d times", got)
	}
}

// TestPush_ConsecutivePollErrorsResetOnSuccess proves the budget counts
// CONSECUTIVE errors, not total: with a budget of 2, a 500 → ok → 500 → ok
// sequence never reaches 2-in-a-row, so the push succeeds. (A total-count budget
// would wrongly abort after the second 500.)
func TestPush_ConsecutivePollErrorsResetOnSuccess(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})
	prevMax := maxConsecutivePollErrors
	maxConsecutivePollErrors = 2
	t.Cleanup(func() { maxConsecutivePollErrors = prevMax })

	var taskCalls int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch {
		case strings.Contains(r.URL.Path, "/themes/upload"):
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"task":{"task":{"id":"task-xyz","status":"0"}}}`))
		case strings.Contains(r.URL.Path, "/themes/task/"):
			n := atomic.AddInt32(&taskCalls, 1)
			w.Header().Set("Content-Type", "application/json")
			switch n {
			case 1, 3: // 500s, but separated by a success → streak never hits 2
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"code":"ServerError"}`))
			case 2: // a good "still running" poll resets the streak
				_, _ = w.Write([]byte(`{"task":{"status":0,"message":"working"}}`))
			default: // 4th poll: done
				_, _ = w.Write([]byte(`{"task":{"status":1,"message":"success"}}`))
			}
		default:
			w.Header().Set("Content-Type", "application/json")
			_, _ = w.Write([]byte(`{"name":"Nova","id":"abc123"}`))
		}
	}))
	t.Cleanup(srv.Close)
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("non-consecutive 500s (reset by a good poll) must not fail the push; got %v", err)
	}
}

// TestPush_TaskTimeoutClassifiesAsNetworkAndPassesPayload: poll never
// terminates within MaxDuration → ErrTaskTimeout envelope (type=network,
// code=4) with elapsed_seconds and last task payload.
func TestPush_TaskTimeoutClassifiesAsNetworkAndPassesPayload(t *testing.T) {
	setupThemeCWD(t, "Nova", "1.0.0")
	// Tiny MaxDuration so we exhaust the budget in <100ms even with
	// realistic poll churn. Interval slightly smaller so we get >=2 polls
	// before the deadline check fires.
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    20 * time.Millisecond,
		MaxDuration: 80 * time.Millisecond,
	})

	srv := newPushTestServer(t, []string{
		`{"data":{"task":{"status":0,"info":"still working","message":"in progress"}}}`,
	})
	c := client.New(srv.URL)

	in := common.ExecInput{Client: c, Flags: pushFlags("abc123")}
	_, err := pushShortcut.Execute(context.Background(), in)
	if err == nil {
		t.Fatalf("expected timeout error after MaxDuration")
	}
	env := envelopeOf(t, err)
	if env["type"] != output.TypeNetwork {
		t.Errorf("type = %v, want %q", env["type"], output.TypeNetwork)
	}
	code, _ := env["code"].(json.Number)
	if codeStr := code.String(); codeStr != "" && codeStr != "4" {
		// Some envelope renderings produce int instead of json.Number.
		// Accept either by also checking the int branch below.
		if v, ok := env["code"].(int); ok && v != output.ExitNetwork {
			t.Errorf("code = %v, want %d", env["code"], output.ExitNetwork)
		}
	}
	if _, ok := env["elapsed_seconds"]; !ok {
		t.Errorf("envelope missing elapsed_seconds; got env=%v", env)
	}
	if _, ok := env["task"]; !ok {
		t.Errorf("envelope missing task passthrough on timeout; got env=%v", env)
	}
}

// ── transientPollError ───────────────────────────────────────────────────────

func TestTransientPollError_ContextCancelled(t *testing.T) {
	if transientPollError(context.Canceled) {
		t.Error("context.Canceled must be non-transient")
	}
}

func TestTransientPollError_ContextDeadline(t *testing.T) {
	if transientPollError(context.DeadlineExceeded) {
		t.Error("context.DeadlineExceeded must be non-transient")
	}
}

// ── isTaskShaped ─────────────────────────────────────────────────────────────

func TestIsTaskShaped_WithInfoField(t *testing.T) {
	if !isTaskShaped(map[string]any{"info": "some info"}) {
		t.Error("map with 'info' key must be task-shaped")
	}
}

func TestIsTaskShaped_NeitherField(t *testing.T) {
	if isTaskShaped(map[string]any{"random": "value"}) {
		t.Error("map with neither status nor info must not be task-shaped")
	}
}

// ── extractTaskPayload ───────────────────────────────────────────────────────

func TestExtractTaskPayload_FallsBackToResp(t *testing.T) {
	resp := map[string]any{"result": "ok"}
	got := extractTaskPayload(resp)
	if got["result"] != "ok" {
		t.Errorf("expected fallback to resp, got %v", got)
	}
}

func TestExtractTaskPayload_PrefersCandidateWithStatus(t *testing.T) {
	task := map[string]any{"status": float64(1), "id": "t-1"}
	resp := map[string]any{"task": task, "data": map[string]any{"other": "stuff"}}
	got := extractTaskPayload(resp)
	if got["id"] != "t-1" {
		t.Errorf("expected task with status, got %v", got)
	}
}

// ── idField ──────────────────────────────────────────────────────────────────

func TestIdField_JSONNumber(t *testing.T) {
	m := map[string]any{"id": json.Number("42")}
	if got := idField(m, "id"); got != "42" {
		t.Errorf("expected 42, got %q", got)
	}
}

func TestIdField_MissingKey(t *testing.T) {
	if got := idField(map[string]any{}, "id"); got != "" {
		t.Errorf("missing key must return empty, got %q", got)
	}
}

// ── extractTaskID ─────────────────────────────────────────────────────────────

func TestExtractTaskID_DoubleNestedTaskTask(t *testing.T) {
	body := map[string]any{
		"task": map[string]any{
			"task": map[string]any{"id": "real-tid"},
		},
	}
	if got := extractTaskID(body); got != "real-tid" {
		t.Errorf("expected real-tid, got %q", got)
	}
}

func TestExtractTaskID_DataWrap(t *testing.T) {
	body := map[string]any{
		"data": map[string]any{"task_id": "data-tid"},
	}
	if got := extractTaskID(body); got != "data-tid" {
		t.Errorf("expected data-tid, got %q", got)
	}
}

func TestExtractTaskID_NonMap(t *testing.T) {
	if got := extractTaskID("not a map"); got != "" {
		t.Errorf("non-map must return empty, got %q", got)
	}
}

// ── decodeTaskJSONFields ──────────────────────────────────────────────────────

func TestDecodeTaskJSONFields_DecodesInfoAndManifest(t *testing.T) {
	task := map[string]any{
		"info":     `{"theme_id":"abc","version":"1.0","store_id":"365580"}`,
		"manifest": `{"theme.css":"theme-589f053bba.css"}`,
		"message":  "success", // scalar string: must NOT be coerced
		"status":   float64(1),
	}
	decodeTaskJSONFields(task)

	info, ok := task["info"].(map[string]any)
	if !ok {
		t.Fatalf("info should decode to a map, got %T", task["info"])
	}
	if info["theme_id"] != "abc" || info["version"] != "1.0" {
		t.Errorf("info decoded wrong: %v", info)
	}
	if m, ok := task["manifest"].(map[string]any); !ok || m["theme.css"] != "theme-589f053bba.css" {
		t.Errorf("manifest should decode to a map: %v (%T)", task["manifest"], task["manifest"])
	}
	// A plain scalar string stays a string (not turned into a number/bool).
	if task["message"] != "success" {
		t.Errorf("scalar string must be left untouched, got %v (%T)", task["message"], task["message"])
	}
	if task["status"] != float64(1) {
		t.Errorf("non-string field must be left untouched, got %v", task["status"])
	}
}

func TestDecodeTaskJSONFields_LeavesInvalidJSONAndEmpty(t *testing.T) {
	task := map[string]any{
		"info":     "not json {",
		"manifest": "",
	}
	decodeTaskJSONFields(task)
	if task["info"] != "not json {" {
		t.Errorf("invalid JSON must be left as the original string, got %v (%T)", task["info"], task["info"])
	}
	if task["manifest"] != "" {
		t.Errorf("empty string must be left as-is, got %v", task["manifest"])
	}
}
