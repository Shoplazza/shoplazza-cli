//go:build e2e

// Package e2e contains end-to-end tests for the themes command that exercise
// a real Shoplazza dev-environment store. They require SHOPLAZZA_STORE and
// SHOPLAZZA_TOKEN env vars and skip cleanly when those are unset.
//
// Build/run with:
//
//	go test -tags=e2e ./tests/e2e/...
//
// Without the e2e build tag these tests are invisible to `go test ./...`.
package e2e

import (
	"context"
	"encoding/json"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func skipIfNoEnv(t *testing.T) {
	t.Helper()
	for _, v := range []string{"SHOPLAZZA_STORE", "SHOPLAZZA_TOKEN"} {
		if os.Getenv(v) == "" {
			t.Skipf("%s not set; skipping e2e", v)
		}
	}
}

func runCLI(t *testing.T, dir string, args ...string) (string, string, error) {
	t.Helper()
	cmd := exec.Command("shoplazza", args...)
	cmd.Dir = dir
	var stdout, stderr strings.Builder
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	return stdout.String(), stderr.String(), err
}

// TestE2E_FullLoop_InitPackagePushPull exercises init → package → list → push → pull.
// Requires an existing disposable test theme; reads SHOPLAZZA_TEST_THEME_ID env var.
func TestE2E_FullLoop_InitPackagePushPull(t *testing.T) {
	skipIfNoEnv(t)
	tmp := t.TempDir()
	t.Chdir(tmp)

	// 1. init
	if _, _, err := runCLI(t, tmp, "themes", "init", "--name", "e2e-shop"); err != nil {
		t.Fatalf("init: %v", err)
	}
	if _, err := os.Stat(filepath.Join(tmp, "e2e-shop", "config")); err != nil {
		t.Fatalf("init did not create theme dir: %v", err)
	}

	// 2. package
	target := filepath.Join(tmp, "e2e-shop")
	if _, _, err := runCLI(t, target, "themes", "package"); err != nil {
		t.Fatalf("package: %v", err)
	}

	// 3. list to find a disposable theme ID
	out, _, err := runCLI(t, target, "themes", "list", "--format", "json")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	themeID := findTestThemeID(t, out)
	if themeID == "" {
		t.Skipf("no SHOPLAZZA_TEST_THEME_ID env var and list output had no disposable theme; skipping push/pull")
	}

	// 4. push (CAREFUL: target the disposable theme, never production)
	if _, _, err := runCLI(t, target, "themes", "push", "--theme-id", themeID); err != nil {
		t.Fatalf("push: %v", err)
	}

	// 5. pull into a fresh dir
	pulled := filepath.Join(tmp, "pulled")
	if err := os.MkdirAll(pulled, 0o755); err != nil {
		t.Fatal(err)
	}
	if _, _, err := runCLI(t, pulled, "themes", "pull", "--theme-id", themeID); err != nil {
		t.Fatalf("pull: %v", err)
	}
}

func TestE2E_PublishDeleteCycle(t *testing.T) {
	skipIfNoEnv(t)
	// dynamic publish/delete via shop themes — placeholder; instrumented when
	// CI provides a disposable theme provisioning workflow.
	_ = context.Background()
	t.Skip("requires disposable theme provisioned; instrument with CI secrets")
}

func TestE2E_Serve_LivereloadBroadcast(t *testing.T) {
	skipIfNoEnv(t)
	// best-effort: launch serve, modify file, observe livereload broadcast
	// over WS via the coder/websocket client.
	t.Skip("requires WS client + ephemeral livereload port wiring; instrument later")
}

func findTestThemeID(t *testing.T, jsonOutput string) string {
	t.Helper()
	if id := os.Getenv("SHOPLAZZA_TEST_THEME_ID"); id != "" {
		return id
	}
	// Parse the list output to find a non-default theme. The envelope may carry
	// the array at the root "themes" key, under "data", or as a bare "data"
	// array, so probe all three. is_default is not always present, so fall back
	// to matching disposable themes by name.
	var parsed map[string]any
	if err := json.Unmarshal([]byte(jsonOutput), &parsed); err != nil {
		return ""
	}
	themes, _ := parsed["themes"].([]any)
	if themes == nil {
		if data, ok := parsed["data"].(map[string]any); ok {
			themes, _ = data["themes"].([]any)
		} else if arr, ok := parsed["data"].([]any); ok {
			themes = arr
		}
	}
	for _, item := range themes {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		if m["is_default"] == true {
			continue
		}
		name, _ := m["name"].(string)
		if strings.Contains(strings.ToLower(name), "test") || strings.Contains(strings.ToLower(name), "disposable") {
			if id, ok := m["id"].(string); ok {
				return id
			}
		}
	}
	return ""
}
