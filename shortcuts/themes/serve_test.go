package themes

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"shoplazza-cli-v2/internal/asynctask"
	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/theme/devstate"
	"shoplazza-cli-v2/internal/theme/doc"
	"shoplazza-cli-v2/internal/theme/pack"
	"shoplazza-cli-v2/internal/theme/watch"
	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// serveFlags builds a FlagSet over a cobra command with both --theme-id and
// --port. Mirrors pushFlags / shareFlags. Passing themeID=""
// selects development-theme mode (create-or-reuse a per-directory dev theme).
func serveFlags(themeID string, port int) common.FlagSet {
	cmd := &cobra.Command{Use: "serve"}
	cmd.Flags().StringP("theme-id", "t", themeID, "")
	cmd.Flags().Int("port", port, "")
	return common.NewCobraFlagSet(cmd)
}

// ─────────── development-theme mode (--theme-id omitted) ───────────

// recordingHandler wraps an http.HandlerFunc and records "<METHOD> <path>?<query>"
// lines for post-run assertions. Thread-safe (serve fires requests from the
// watcher goroutine too).
type recordingHandler struct {
	mu    sync.Mutex
	lines []string
	next  http.HandlerFunc
}

func (h *recordingHandler) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	h.mu.Lock()
	h.lines = append(h.lines, r.Method+" "+r.URL.Path+"?"+r.URL.RawQuery)
	h.mu.Unlock()
	h.next(w, r)
}

func (h *recordingHandler) requests() []string {
	h.mu.Lock()
	defer h.mu.Unlock()
	return append([]string(nil), h.lines...)
}

// uploadThemeIDOf parses a recorded "<METHOD> <path>?<query>" line and
// returns (theme_id value, true) for /themes/upload requests. The client
// omits empty params from the query string, so a create-new-theme upload
// reports ("", true) — theme_id absent — rather than "theme_id=&".
func uploadThemeIDOf(line string) (string, bool) {
	if !strings.Contains(line, "/themes/upload") {
		return "", false
	}
	_, rawQuery, _ := strings.Cut(line, "?")
	q, err := url.ParseQuery(rawQuery)
	if err != nil {
		return "", false
	}
	return q.Get("theme_id"), true
}

// runServeBriefly executes serve with the given client and flags, lets it
// run until the watcher is up, then cancels and waits for a clean exit.
func runServeBriefly(t *testing.T, c *client.Client, fs common.FlagSet) error {
	t.Helper()
	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := serveShortcut.Execute(ctx, common.ExecInput{Client: c, Flags: fs})
		done <- e
	}()
	time.Sleep(500 * time.Millisecond)
	cancel()
	select {
	case err := <-done:
		return err
	case <-time.After(3 * time.Second):
		t.Fatal("serve did not exit within 3s after ctx cancel")
		return nil
	}
}

// TestServe_NoThemeID_CreatesDevThemeAndSavesState: with --theme-id omitted
// and no prior state, serve must create a development theme (upload with an
// empty theme_id query), persist the returned id in
// .shoplazza/theme-state.json keyed by store host, and run the rest of the
// pipeline (doctree) against the new id.
func TestServe_NoThemeID_CreatesDevThemeAndSavesState(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)
	withPushPollOpts(t, asynctask.PollOptions{Interval: time.Millisecond, MaxDuration: 5 * time.Second})

	rec := &recordingHandler{next: func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		if strings.Contains(r.URL.Path, "/themes/upload") {
			// Sync echo: server allocates dev theme id immediately.
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"theme_id": "dev1"}})
			return
		}
		mockServeOK(w, r)
	}}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	if err := runServeBriefly(t, client.New(srv.URL), serveFlags("", 0)); err != nil {
		t.Fatalf("serve err: %v", err)
	}

	// State written, keyed by the store host.
	id, ok := devstate.Load(dir, devstate.StoreKey(srv.URL))
	if !ok || id != "dev1" {
		t.Errorf("devstate after serve = (%q, %v), want (\"dev1\", true)", id, ok)
	}

	var sawCreateUpload, sawDocTree bool
	for _, line := range rec.requests() {
		if id, isUpload := uploadThemeIDOf(line); isUpload && id == "" {
			sawCreateUpload = true // empty/absent theme_id → create-new-theme upload
		}
		if strings.Contains(line, "/themes/dev1/doctree") {
			sawDocTree = true
		}
	}
	if !sawCreateUpload {
		t.Errorf("expected an upload with empty theme_id (create dev theme); requests:\n%s",
			strings.Join(rec.requests(), "\n"))
	}
	if !sawDocTree {
		t.Errorf("expected doctree fetch for the new dev theme id; requests:\n%s",
			strings.Join(rec.requests(), "\n"))
	}
}

// TestServe_NoThemeID_ReusesSavedDevTheme: a saved dev theme id that still
// exists remotely must be reused — serve runs the normal initial-push
// pipeline against it and does NOT create a new theme.
func TestServe_NoThemeID_ReusesSavedDevTheme(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)
	withPushPollOpts(t, asynctask.PollOptions{Interval: time.Millisecond, MaxDuration: 5 * time.Second})

	rec := &recordingHandler{next: mockServeOK}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	if err := devstate.Save(dir, devstate.StoreKey(srv.URL), "dev1"); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	if err := runServeBriefly(t, client.New(srv.URL), serveFlags("", 0)); err != nil {
		t.Fatalf("serve err: %v", err)
	}

	var sawReuseUpload, sawCreateUpload bool
	for _, line := range rec.requests() {
		if id, isUpload := uploadThemeIDOf(line); isUpload {
			if id == "dev1" {
				sawReuseUpload = true
			}
			if id == "" {
				sawCreateUpload = true
			}
		}
	}
	if !sawReuseUpload {
		t.Errorf("expected initial push targeting saved dev theme dev1; requests:\n%s",
			strings.Join(rec.requests(), "\n"))
	}
	if sawCreateUpload {
		t.Errorf("must NOT create a new theme when the saved one exists; requests:\n%s",
			strings.Join(rec.requests(), "\n"))
	}
	if id, _ := devstate.Load(dir, devstate.StoreKey(srv.URL)); id != "dev1" {
		t.Errorf("state must keep dev1, got %q", id)
	}
}

// TestServe_NoThemeID_RecreatesWhenSavedThemeGone: when the saved dev theme
// was deleted remotely (detail GET → 404), serve must fall back to creating
// a fresh dev theme and overwrite the stale state entry.
func TestServe_NoThemeID_RecreatesWhenSavedThemeGone(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)
	withPushPollOpts(t, asynctask.PollOptions{Interval: time.Millisecond, MaxDuration: 5 * time.Second})

	rec := &recordingHandler{next: func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.Contains(r.URL.Path, "/themes/dev0"):
			http.Error(w, `{"code":"NotFound","message":"theme gone"}`, http.StatusNotFound)
		case strings.Contains(r.URL.Path, "/themes/upload"):
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"theme_id": "dev2"}})
		default:
			mockServeOK(w, r)
		}
	}}
	srv := httptest.NewServer(rec)
	t.Cleanup(srv.Close)

	if err := devstate.Save(dir, devstate.StoreKey(srv.URL), "dev0"); err != nil {
		t.Fatalf("seed state: %v", err)
	}

	if err := runServeBriefly(t, client.New(srv.URL), serveFlags("", 0)); err != nil {
		t.Fatalf("serve err: %v", err)
	}

	if id, ok := devstate.Load(dir, devstate.StoreKey(srv.URL)); !ok || id != "dev2" {
		t.Errorf("state after recreate = (%q, %v), want (\"dev2\", true)", id, ok)
	}
	var sawDocTreeDev2 bool
	for _, line := range rec.requests() {
		if strings.Contains(line, "/themes/dev2/doctree") {
			sawDocTreeDev2 = true
		}
	}
	if !sawDocTreeDev2 {
		t.Errorf("pipeline must continue against the recreated theme dev2; requests:\n%s",
			strings.Join(rec.requests(), "\n"))
	}
}

// TestServe_DryRun_DevModeNoState: dry-run without --theme-id and without
// saved state must preview the create path — an upload with empty theme_id,
// the task poll, and a doctree fetch with a placeholder id. No detail plan
// (there is no theme to verify yet), no state write.
func TestServe_DryRun_DevModeNoState(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	res, err := serveShortcut.Execute(context.Background(), common.ExecInput{
		DryRun: true,
		Flags:  serveFlags("", 21647),
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if len(res.Plans) != 3 {
		paths := make([]string, 0, len(res.Plans))
		for _, p := range res.Plans {
			paths = append(paths, p.Path)
		}
		t.Fatalf("dev-mode dry-run should emit 3 plans (create-upload + task + doctree); got %d: %v",
			len(res.Plans), paths)
	}
	var sawCreateUpload, sawTask, sawDocTree bool
	for _, p := range res.Plans {
		switch {
		case strings.Contains(p.Path, "/themes/upload"):
			if id, _ := p.Query["theme_id"].(string); id == "" {
				sawCreateUpload = true
			}
		case strings.Contains(p.Path, "/themes/task/"):
			sawTask = true
		case strings.Contains(p.Path, "/doctree"):
			sawDocTree = true
		}
	}
	if !sawCreateUpload || !sawTask || !sawDocTree {
		t.Errorf("missing plan kinds: createUpload=%v task=%v doctree=%v", sawCreateUpload, sawTask, sawDocTree)
	}
	if _, ok := devstate.Load(dir, "default"); ok {
		t.Error("dry-run must not write state")
	}
}

// TestServe_DryRunDoesNotStartWatcherOrServer: dry-run emits the initial
// detail + upload + task-poll + doctree plans and does NOT touch the
// filesystem, bind any port, or open any watcher.
func TestServe_DryRunDoesNotStartWatcherOrServer(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	res, err := serveShortcut.Execute(context.Background(), common.ExecInput{
		DryRun: true,
		Flags:  serveFlags("abc", 21647),
	})
	if err != nil {
		t.Fatalf("dry-run err: %v", err)
	}
	if len(res.Plans) != 4 {
		t.Fatalf("dry-run should emit 4 plans (detail + upload + task + doctree); got %d", len(res.Plans))
	}
	// Verify the four plan kinds are all present.
	hasDetail, hasUpload, hasTask, hasDoc := false, false, false, false
	for _, p := range res.Plans {
		switch {
		case strings.Contains(p.Path, "/doctree"):
			hasDoc = true
		case strings.Contains(p.Path, "/2020-07/themes/upload"):
			hasUpload = true
		case strings.Contains(p.Path, "/2026-01/themes/task/"):
			hasTask = true
		case strings.Contains(p.Path, "/2026-01/themes/abc") &&
			!strings.Contains(p.Path, "/doc") &&
			!strings.Contains(p.Path, "/task"):
			hasDetail = true
		}
	}
	if !hasDetail || !hasUpload || !hasTask || !hasDoc {
		paths := make([]string, 0, len(res.Plans))
		for _, p := range res.Plans {
			paths = append(paths, p.Path)
		}
		t.Fatalf("expected detail + upload + task + doctree plans; got paths=%v", paths)
	}
}

// TestServe_DoesNotModifyThemeFiles: snapshot every file's mtime before and
// after a brief serve run. The CLI must signal browser refresh via the
// livereload extension, never write to a theme file. Cancelling the context
// is the cleanest shutdown signal.
func TestServe_DoesNotModifyThemeFiles(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	// Use tiny push poll interval so the initial push completes quickly.
	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})

	beforeMtime, err := dirSnapshot(dir)
	if err != nil {
		t.Fatalf("before snapshot: %v", err)
	}

	srv := newServeMockServer(t)

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := serveShortcut.Execute(ctx, common.ExecInput{
			Client: client.New(srv.URL),
			Flags:  serveFlags("abc", 0),
		})
		done <- e
	}()
	// Give serve time to do its initial push + doctree + start watcher.
	time.Sleep(500 * time.Millisecond)
	cancel()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("serve did not exit within 3s after ctx cancel")
	}

	afterMtime, err := dirSnapshot(dir)
	if err != nil {
		t.Fatalf("after snapshot: %v", err)
	}
	// Every file present in `before` must still exist and have the same
	// mtime. Test does NOT assert on extra files: push.go creates a tmp
	// zip in cwd via filepath.Abs on a relative name, but the zip is
	// deferred-deleted before push.Execute returns to serve's caller, so
	// in practice it's gone by the time we snapshot.
	for k, v := range beforeMtime {
		af, ok := afterMtime[k]
		if !ok {
			t.Errorf("file %s disappeared during serve (CLI must not delete)", k)
			continue
		}
		if !af.Equal(v) {
			t.Errorf("file %s mtime changed during serve (CLI must not write); %v -> %v", k, v, af)
		}
	}
}

// TestServe_HTTPSyncFailureKeepsWatching: when an HTTP sync (POST / PATCH /
// DELETE on /doc) returns 500, serve must add the file to pendingFailures,
// log a warning, and KEEP RUNNING. We verify the watcher is still alive by
// cancelling the context and observing a clean exit.
func TestServe_HTTPSyncFailureKeepsWatching(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	withPushPollOpts(t, asynctask.PollOptions{
		Interval:    1 * time.Millisecond,
		MaxDuration: 5 * time.Second,
	})

	// Server returns 500 on any /doc POST or PATCH or DELETE; OK on
	// everything else (so the initial push + doctree succeed).
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/doc") &&
			(r.Method == http.MethodPost || r.Method == http.MethodPatch || r.Method == http.MethodDelete) {
			http.Error(w, `{"code":"BoomError","message":"boom"}`, 500)
			return
		}
		mockServeOK(w, r)
	}))
	defer srv.Close()

	// Redirect os.Stderr to a pipe so we can capture warn lines emitted by
	// handleSync.
	oldStderr := os.Stderr
	pr, pw, _ := os.Pipe()
	os.Stderr = pw
	t.Cleanup(func() { os.Stderr = oldStderr })

	stderrCh := make(chan string, 1)
	go func() {
		b, _ := io.ReadAll(pr)
		stderrCh <- string(b)
	}()

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, e := serveShortcut.Execute(ctx, common.ExecInput{
			Client: client.New(srv.URL),
			Flags:  serveFlags("abc", 0),
		})
		done <- e
	}()

	// Allow time for initial push + doctree + watcher startup.
	time.Sleep(600 * time.Millisecond)
	// Modify an existing file under the theme tree. The watcher will fire
	// OnUpdate; the snapshot returned by mockServeOK's doctree shape may or
	// may not contain assets/main.css depending on doc.FromDocTreeResponse
	// envelope handling — either way, the POST or PATCH hits the failing
	// /doc handler and the file lands in pendingFailures.
	target := filepath.Join(dir, "assets", "main.css")
	if err := os.WriteFile(target, []byte("changed"), 0o644); err != nil {
		t.Fatalf("WriteFile: %v", err)
	}
	// Watcher debounce is 50ms + 25ms tick interval; allow ample slack.
	time.Sleep(800 * time.Millisecond)

	// Cancel context — serve must exit cleanly without panicking.
	cancel()
	pw.Close()
	select {
	case <-done:
	case <-time.After(3 * time.Second):
		t.Fatalf("serve did not exit within 3s after ctx cancel; sync failure may have escaped to fatal")
	}
	captured := <-stderrCh
	os.Stderr = oldStderr

	// Assert the unsynced marker is present. We do NOT assert on a
	// specific event kind ([create]/[update]/[delete]) — depending on the
	// race between doctree snapshot construction and the file-modification
	// trigger, either path may log.
	if !strings.Contains(captured, "unsynced:") {
		t.Errorf("expected '[unsynced: N]' marker in stderr after failed sync; captured:\n%s", captured)
	}
}

// ─────────── helpers ───────────

// dirSnapshot returns a map of relpath -> mtime for every regular file
// under dir (no directories, no symlinks). Used to assert that serve
// never writes to any theme file.
func dirSnapshot(dir string) (map[string]time.Time, error) {
	out := map[string]time.Time{}
	err := filepath.Walk(dir, func(p string, info os.FileInfo, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if info.IsDir() {
			return nil
		}
		rel, err := filepath.Rel(dir, p)
		if err != nil {
			return err
		}
		out[rel] = info.ModTime()
		return nil
	})
	return out, err
}

// newServeMockServer is a thin wrapper around httptest.NewServer using
// mockServeOK as the universal handler. Suitable for the happy-path test.
func newServeMockServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(mockServeOK))
	t.Cleanup(srv.Close)
	return srv
}

// mockServeOK routes the endpoint families serve depends on:
//   - GET  /openapi/2026-01/themes/{id}          → detail (push step 1)
//   - POST /openapi/2020-07/themes/upload        → upload, returns task_id
//   - GET  /openapi/2026-01/themes/task/{id}     → poll, returns status=1
//   - GET  /openapi/2026-01/themes/{id}/doctree  → doctree
//   - GET  /openapi/2026-01/shop                 → banner domain lookup
//
// Everything else returns 200 with {"data":{}} so the mock degrades safely.
func mockServeOK(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Content-Type", "application/json")
	switch {
	case strings.Contains(r.URL.Path, "/doctree"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"doctree": map[string]any{
					"layout": []any{"theme.liquid"},
					"assets": []any{"main.css"},
				},
			},
		})
	case strings.Contains(r.URL.Path, "/themes/upload"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"task_id": "t1"},
		})
	case strings.Contains(r.URL.Path, "/themes/task/"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{
				"task": map[string]any{"task_id": "t1", "status": 1, "info": "done"},
			},
		})
	case strings.Contains(r.URL.Path, "/shop"):
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"shop": map[string]any{"domain": "test.myshoplaza.com"}},
		})
	case strings.Contains(r.URL.Path, "/themes/"):
		// Theme detail.
		_ = json.NewEncoder(w).Encode(map[string]any{
			"data": map[string]any{"name": "TestTheme", "id": "abc"},
		})
	default:
		_, _ = w.Write([]byte(`{"data":{}}`))
	}
}

// ── handleSync ────────────────────────────────────────────────────────────────

// recordDocMethods returns a server that 200s everything and records the HTTP
// method of every /doc request handleSync makes.
func recordDocMethods(t *testing.T) (*httptest.Server, *[]string) {
	t.Helper()
	var methods []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if strings.Contains(r.URL.Path, "/doc") {
			methods = append(methods, r.Method)
		}
		_, _ = w.Write([]byte(`{"ok":true}`))
	}))
	t.Cleanup(srv.Close)
	return srv, &methods
}

// TestHandleSync_CreateEventOnExistingFile_Patches: an fsnotify "create" event
// for a doc ALREADY on the server (atomic-save renames-into-place look like
// creates) must PATCH — NOT POST a create-stub for an existing doc, which the
// server 500s.
func TestHandleSync_CreateEventOnExistingFile_Patches(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "config"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "config", "settings_data.json"), []byte(`{}`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	srv, methods := recordDocMethods(t)
	snap := doc.FileSnapshot{}
	snap.Add("config", "settings_data.json")

	handleSync(context.Background(), client.New(srv.URL), "tid", "create",
		"config/settings_data.json", &snap, doc.NewDeduper(), newPendingFailures(), watch.NewLiveReloadServer(0), io.Discard)

	for _, m := range *methods {
		if m == http.MethodPost {
			t.Fatalf("create event on an existing doc must NOT POST a create-stub; methods=%v", *methods)
		}
	}
	if len(*methods) == 0 || (*methods)[len(*methods)-1] != http.MethodPatch {
		t.Fatalf("expected a PATCH for the existing doc; methods=%v", *methods)
	}
}

// TestHandleSync_CreateEventOnNewFile_PostsThenPatches: a genuinely new file
// (absent from the snapshot) still creates the stub then PATCHes content.
func TestHandleSync_CreateEventOnNewFile_PostsThenPatches(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "blocks"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "blocks", "new.liquid"), []byte(`<div></div>`), 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	srv, methods := recordDocMethods(t)
	snap := doc.FileSnapshot{}

	handleSync(context.Background(), client.New(srv.URL), "tid", "create",
		"blocks/new.liquid", &snap, doc.NewDeduper(), newPendingFailures(), watch.NewLiveReloadServer(0), io.Discard)

	if len(*methods) < 2 || (*methods)[0] != http.MethodPost || (*methods)[1] != http.MethodPatch {
		t.Fatalf("new file should POST-stub then PATCH; methods=%v", *methods)
	}
	if !snap.Has("blocks", "new.liquid") {
		t.Fatal("new file should be added to the snapshot after create")
	}
}

// TestHandleSync_BinaryContentSkipped: invalid UTF-8 (binary assets) must
// NEVER ride the JSON doc patch — every non-UTF-8 byte would be mangled to
// U+FFFD on the wire. handleSync skips the file, makes NO doc API call, and
// logs a one-line [skip] warning pointing at `themes push`.
func TestHandleSync_BinaryContentSkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets"), 0o755); err != nil {
		t.Fatal(err)
	}
	binary := []byte{0x89, 0x50, 0x4e, 0x47, 0xff, 0xfe, 0x00, 0x80} // PNG-ish, invalid UTF-8
	if err := os.WriteFile(filepath.Join(dir, "assets", "logo.png"), binary, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	srv, methods := recordDocMethods(t)
	snap := doc.FileSnapshot{}
	var log strings.Builder

	for _, kind := range []string{"create", "update"} {
		handleSync(context.Background(), client.New(srv.URL), "tid", kind,
			"assets/logo.png", &snap, doc.NewDeduper(), newPendingFailures(), watch.NewLiveReloadServer(0), &log)
	}

	if len(*methods) != 0 {
		t.Errorf("binary file must not hit the doc API; methods=%v", *methods)
	}
	if !strings.Contains(log.String(), "[skip]") || !strings.Contains(log.String(), "binary") {
		t.Errorf("expected a [skip] ... binary warning, got: %q", log.String())
	}
	if !strings.Contains(log.String(), "themes push") {
		t.Errorf("warning should point at `themes push`, got: %q", log.String())
	}
}

// TestHandleSync_VanishedFileSkipped: when the file disappears between the
// fsnotify event and handleSync, nothing must be pushed (previously the empty
// content of the failed read truncated the remote file) and a note is logged.
func TestHandleSync_VanishedFileSkipped(t *testing.T) {
	dir := t.TempDir()
	t.Chdir(dir)

	srv, methods := recordDocMethods(t)
	snap := doc.FileSnapshot{}
	snap.Add("assets", "gone.css")
	var log strings.Builder

	handleSync(context.Background(), client.New(srv.URL), "tid", "update",
		"assets/gone.css", &snap, doc.NewDeduper(), newPendingFailures(), watch.NewLiveReloadServer(0), &log)

	if len(*methods) != 0 {
		t.Errorf("vanished file must not hit the doc API; methods=%v", *methods)
	}
	if !strings.Contains(log.String(), "[skip]") {
		t.Errorf("expected a [skip] note for the vanished file, got: %q", log.String())
	}
}

// TestHandleSync_DirectorySkipped: belt-and-braces — even if a directory
// event slips past the watcher's suppression, handleSync must not create a
// phantom remote doc named after the directory.
func TestHandleSync_DirectorySkipped(t *testing.T) {
	dir := t.TempDir()
	if err := os.MkdirAll(filepath.Join(dir, "assets", "icons"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)

	srv, methods := recordDocMethods(t)
	snap := doc.FileSnapshot{}

	handleSync(context.Background(), client.New(srv.URL), "tid", "create",
		"assets/icons", &snap, doc.NewDeduper(), newPendingFailures(), watch.NewLiveReloadServer(0), io.Discard)

	if len(*methods) != 0 {
		t.Errorf("directory must not hit the doc API; methods=%v", *methods)
	}
	if snap.Has("assets", "icons") {
		t.Error("directory must not be added to the snapshot")
	}
}

// TestBuildWatchFilter_HonorsThemeignore: serve's watch filter must apply the
// same .themeignore rules push does — an ignored file edited during serve was
// previously pushed anyway.
func TestBuildWatchFilter_HonorsThemeignore(t *testing.T) {
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, ".themeignore"),
		[]byte("assets/ignored.css\nsections/draft-*.liquid\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	ignorer, err := pack.LoadThemeIgnorer(dir, "")
	if err != nil {
		t.Fatalf("LoadThemeIgnorer: %v", err)
	}
	if ignorer == nil {
		t.Fatal("expected a non-nil ignorer when .themeignore exists")
	}
	filter := buildWatchFilter(ignorer)

	if filter("assets/ignored.css") {
		t.Error(".themeignore'd file must be filtered out")
	}
	if filter("sections/draft-x.liquid") {
		t.Error(".themeignore glob must be honored")
	}
	if !filter("assets/main.css") {
		t.Error("non-ignored theme file must pass the filter")
	}
	if filter("config/settings_data.json.sb-123") {
		t.Error("editor temp artifacts must still be filtered")
	}
	if filter("README.md") {
		t.Error("non-theme-tree files must still be filtered")
	}

	// Nil ignorer (no .themeignore) keeps the original behavior.
	noIgnore := buildWatchFilter(nil)
	if !noIgnore("assets/ignored.css") {
		t.Error("without .themeignore the file must pass")
	}
}

// TestServe_InvalidLiveReloadPortIsValidationError: out-of-range ports must
// fail fast as validation (exit 2) — previously 99999 surfaced as a
// network-class bind failure hinting "another instance may be running".
func TestServe_InvalidLiveReloadPortIsValidationError(t *testing.T) {
	for _, port := range []int{-1, 65536, 99999} {
		_, err := serveShortcut.Execute(context.Background(), common.ExecInput{
			Flags: serveFlags("abc", port),
		})
		if err == nil {
			t.Fatalf("port %d: expected validation error", port)
		}
		type envelopeCarrier interface{ Envelope() map[string]any }
		var ec envelopeCarrier
		if !errors.As(err, &ec) {
			t.Fatalf("port %d: error does not implement Envelope(); got %T", port, err)
		}
		env := ec.Envelope()
		if env["type"] != output.TypeValidation {
			t.Errorf("port %d: type = %v, want validation", port, env["type"])
		}
	}
}

// TestServe_InvalidThemeIDIsValidationError: --theme-id is spliced into URL
// paths; junk like "../x" must be rejected up front.
func TestServe_InvalidThemeIDIsValidationError(t *testing.T) {
	_, err := serveShortcut.Execute(context.Background(), common.ExecInput{
		Flags: serveFlags("../x", 0),
	})
	if err == nil {
		t.Fatal("expected validation error for malformed theme id")
	}
	type envelopeCarrier interface{ Envelope() map[string]any }
	var ec envelopeCarrier
	if !errors.As(err, &ec) {
		t.Fatalf("error does not implement Envelope(); got %T", err)
	}
	if env := ec.Envelope(); env["type"] != output.TypeValidation {
		t.Errorf("type = %v, want validation", env["type"])
	}
}

// TestIsEditorTempFiltersAtomicSaveArtifacts: the watcher must skip editor
// temp/swap/hidden files.
func TestIsEditorTempFiltersAtomicSaveArtifacts(t *testing.T) {
	temps := []string{
		"config/settings_data.json.sb-1035804a-srGLvL",
		"layout/theme.liquid~",
		"assets/app.css.swp",
		"config/.DS_Store",
		"blocks/x.tmp",
	}
	for _, f := range temps {
		if !doc.IsEditorTemp(f) {
			t.Errorf("isEditorTemp(%q) = false, want true (should be skipped)", f)
		}
	}
	reals := []string{"config/settings_data.json", "layout/theme.liquid", "assets/app.css", "snippets/foo.liquid"}
	for _, f := range reals {
		if doc.IsEditorTemp(f) {
			t.Errorf("isEditorTemp(%q) = true, want false (real theme file)", f)
		}
	}
}
