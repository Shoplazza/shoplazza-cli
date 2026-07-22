package tests_test

import (
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/testenv"
)

// metaEnv isolates the child's config dir (testenv.IsolateConfigDir redirects
// the test process env, which the child inherits via os.Environ) and disables
// the background refresh so no test ever touches the network.
func metaEnv(t *testing.T) []string {
	t.Helper()
	testenv.IsolateConfigDir(t)
	return []string{"SHOPLAZZA_CLI_NO_META_UPDATE=1"}
}

// seedMetaCache writes content as the downloaded-spec cache the child CLI
// will find under its (isolated) user config dir.
func seedMetaCache(t *testing.T, content string) {
	t.Helper()
	base, err := os.UserConfigDir()
	if err != nil {
		t.Fatal(err)
	}
	dir := filepath.Join(base, "shoplazza-cli", "meta")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "cli_meta.json"), []byte(content), 0o600); err != nil {
		t.Fatal(err)
	}
}

// doctorMetadataMessage runs `doctor check` and returns the metadata check's
// message field.
func doctorMetadataMessage(t *testing.T, bin string, env []string) string {
	t.Helper()
	stdout, stderr, code := runCLI(t, bin, env, "doctor", "check")
	if code != 0 {
		t.Fatalf("doctor check exit=%d stderr=%s", code, stderr)
	}
	var body struct {
		Checks []struct {
			Name    string `json:"name"`
			Message string `json:"message"`
		} `json:"checks"`
	}
	if err := json.Unmarshal([]byte(stdout), &body); err != nil {
		t.Fatalf("doctor output not JSON: %v\n%s", err, stdout)
	}
	for _, c := range body.Checks {
		if c.Name == "metadata" {
			return c.Message
		}
	}
	t.Fatalf("no metadata check in doctor output: %s", stdout)
	return ""
}

// A newer cached spec must add its module to the command tree and be reported
// by doctor as the active source.
func TestMetaCache_NewerCacheAddsCommands(t *testing.T) {
	bin := buildBinary(t)
	env := metaEnv(t)
	seedMetaCache(t, `{
		"version": "vE2E",
		"generated_at": "9999-01-01T00:00:00Z",
		"modules": [{
			"name": "zz-e2e-probe",
			"commands": [{
				"id": "zz-e2e-probe-list",
				"command_path": ["list"],
				"summary": "e2e probe command",
				"http": {"method": "GET", "path": "/openapi/2026-01/zz_e2e_probe"}
			}]
		}]
	}`)

	stdout, stderr, code := runCLI(t, bin, env, "zz-e2e-probe", "--help")
	if code != 0 {
		t.Fatalf("probe module missing, exit=%d stderr=%s", code, stderr)
	}
	if !strings.Contains(stdout, "list") {
		t.Fatalf("probe module help should list its command:\n%s", stdout)
	}

	msg := doctorMetadataMessage(t, bin, env)
	if !strings.Contains(msg, "source=cached") || !strings.Contains(msg, "revision=9999-01-01T00:00:00Z") {
		t.Fatalf("doctor should report the cached spec, got %q", msg)
	}
}

// A corrupt cache must be ignored: embedded command tree, embedded source.
func TestMetaCache_CorruptCacheFallsBack(t *testing.T) {
	bin := buildBinary(t)
	env := metaEnv(t)
	seedMetaCache(t, `{not valid json`)

	if _, _, code := runCLI(t, bin, env, "zz-e2e-probe", "--help"); code == 0 {
		t.Fatal("corrupt cache must not register modules")
	}
	// Embedded tree still works.
	stdout, stderr, code := runCLI(t, bin, env, "products", "--help")
	if code != 0 {
		t.Fatalf("embedded fallback broken, exit=%d stderr=%s", code, stderr)
	}
	_ = stdout

	if msg := doctorMetadataMessage(t, bin, env); !strings.Contains(msg, "source=embedded") {
		t.Fatalf("doctor should report embedded fallback, got %q", msg)
	}
}
