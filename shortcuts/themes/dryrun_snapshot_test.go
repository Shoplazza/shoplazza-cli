package themes

// Golden-file snapshot tests for the dry-run output of every theme workflow
// shortcut, locking the Method/Path/Query/Body shape of each PlannedRequest so
// any drift surfaces as a snapshot diff. The first run (or UPDATE_GOLDEN=1)
// writes testdata/<name>.golden.json; later runs byte-compare against it.
// Non-deterministic fields (absolute paths, timestamps) are normalised first.

import (
	"bytes"
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

// testdataDir resolves the testdata directory from this test file's source
// location. Anchoring on runtime.Caller (rather than cwd) keeps goldens in the
// package source tree even when a test t.Chdir()s into a tmp theme dir.
func testdataDir() string {
	_, thisFile, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(thisFile), "testdata")
}

// snapshot serializes got as indented JSON and compares against
// testdata/<name>.golden.json. On first run (file missing) or when
// UPDATE_GOLDEN=1, it writes the current output instead.
func snapshot(t *testing.T, name string, got any) {
	t.Helper()
	b, err := json.MarshalIndent(got, "", "  ")
	if err != nil {
		t.Fatalf("marshal: %v", err)
	}
	dir := testdataDir()
	golden := filepath.Join(dir, name+".golden.json")
	update := os.Getenv("UPDATE_GOLDEN") == "1"
	want, readErr := os.ReadFile(golden)
	if readErr != nil || update {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(golden, b, 0o644); err != nil {
			t.Fatal(err)
		}
		if update {
			t.Logf("updated golden: %s", golden)
		} else {
			t.Logf("wrote new golden: %s - re-run to verify", golden)
		}
		return
	}
	if !bytes.Equal(bytes.TrimSpace(want), bytes.TrimSpace(b)) {
		t.Errorf("snapshot drift for %s.\nWANT:\n%s\nGOT:\n%s", name, want, b)
	}
}

// plansToMap normalises []PlannedRequest into a stable map shape for
// snapshotting, keeping field order deterministic and letting us strip future
// fields without rewriting every golden.
func plansToMap(plans []common.PlannedRequest) map[string]any {
	arr := make([]map[string]any, 0, len(plans))
	for _, p := range plans {
		arr = append(arr, map[string]any{
			"method": p.Method,
			"path":   p.Path,
			"query":  p.Query,
			"body":   p.Body,
		})
	}
	return map[string]any{"plans": arr}
}

// TestSnapshot_InitDryRun locks the init dry-run Body shape. Stderr is
// captured to swallow the cd hint; only the structured Body is snapshotted.
func TestSnapshot_InitDryRun(t *testing.T) {
	in := common.ExecInput{
		DryRun: true,
		Flags:  flagsWithName("my-shop"),
	}
	var res common.ExecResult
	var execErr error
	captureStderr(t, func() {
		res, execErr = initShortcut.Execute(context.Background(), in)
	})
	if execErr != nil {
		t.Fatalf("Execute err: %v", execErr)
	}
	snapshot(t, "init_dry_run", res.Body)
}

// TestSnapshot_PackageDryRun locks package's Body. The absolute zip_path
// must be normalised to <TMP> so the golden is reproducible across machines.
func TestSnapshot_PackageDryRun(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	in := common.ExecInput{DryRun: true, Flags: flagsWithNoIgnore(false)}
	var res common.ExecResult
	var execErr error
	captureStderr(t, func() {
		res, execErr = packageShortcut.Execute(context.Background(), in)
	})
	if execErr != nil {
		t.Fatalf("Execute err: %v", execErr)
	}
	// Normalise the absolute tmp path so the snapshot is deterministic.
	if zp, ok := res.Body["zip_path"].(string); ok {
		res.Body["zip_path"] = strings.ReplaceAll(zp, dir, "<TMP>")
	}
	snapshot(t, "package_dry_run", res.Body)
}

// TestSnapshot_PullDryRun locks pull's 2-plan dry-run shape (PlanDetail v2 +
// PlanDownload v1).
func TestSnapshot_PullDryRun(t *testing.T) {
	in := common.ExecInput{DryRun: true, Flags: pullFlags("abc")}
	res, err := pullShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	snapshot(t, "pull_dry_run", plansToMap(res.Plans))
}

// TestSnapshot_PushDryRun locks push's 3-plan dry-run shape (detail + upload
// + task-poll). The task_id is a static placeholder since dry-run never
// hits the upload endpoint.
func TestSnapshot_PushDryRun(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	in := common.ExecInput{DryRun: true, Flags: pushFlags("abc")}
	res, err := pushShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	snapshot(t, "push_dry_run", plansToMap(res.Plans))
}

// TestSnapshot_ShareDryRun locks share's dry-run shape: PlanShareShop +
// PlanShareUpload with theme_id="" (always a fresh temporary theme).
func TestSnapshot_ShareDryRun(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	in := common.ExecInput{DryRun: true, Flags: shareFlags()}
	res, err := shareShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	snapshot(t, "share_dry_run_path_a", plansToMap(res.Plans))
}

// TestSnapshot_ServeDryRun locks serve's 4-plan dry-run shape (detail +
// upload + task-poll + doctree). Watcher and LiveReload server are not
// started; this is a pure preview.
func TestSnapshot_ServeDryRun(t *testing.T) {
	dir := t.TempDir()
	makeThemeAt(t, dir)
	writeSettings(t, dir, "X", "1.0")
	t.Chdir(dir)

	in := common.ExecInput{DryRun: true, Flags: serveFlags("abc", 21647)}
	res, err := serveShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	snapshot(t, "serve_dry_run", plansToMap(res.Plans))
}
