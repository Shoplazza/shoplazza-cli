// Contract smoke suite: enumerates every runnable leaf command in-process via
// cmd.NewRootCmd(), then exercises each one through the real compiled binary.
// Tier 1 checks --help wiring; Tier 2 checks the output contract under
// --dry-run against a mock server.
package tests_test

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/cmd"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"

	"github.com/spf13/cobra"
)

// ── shared binary (built once for the whole package) ─────────────────────────

// baseEnv snapshots the process env before any t.Setenv isolation, so the
// go-build subprocess keeps the real GOCACHE/HOME (a redirected HOME would
// cold-start the build cache).
var baseEnv = os.Environ()

var (
	sharedBinOnce sync.Once
	sharedBinDir  string
	sharedBinPath string
	sharedBinErr  error
)

// sharedBinary builds the CLI once and returns the path; TestMain removes it.
func sharedBinary(t *testing.T) string {
	t.Helper()
	sharedBinOnce.Do(func() {
		sharedBinDir, sharedBinErr = os.MkdirTemp("", "shoplazza-contract-")
		if sharedBinErr != nil {
			return
		}
		sharedBinPath = filepath.Join(sharedBinDir, "shoplazza")
		projectRoot, err := filepath.Abs("..")
		if err != nil {
			sharedBinErr = err
			return
		}
		build := exec.Command("go", "build", "-o", sharedBinPath, ".")
		build.Dir = projectRoot
		build.Env = baseEnv
		if out, err := build.CombinedOutput(); err != nil {
			sharedBinErr = fmt.Errorf("go build: %w\n%s", err, out)
		}
	})
	if sharedBinErr != nil {
		t.Fatalf("build shared binary: %v", sharedBinErr)
	}
	return sharedBinPath
}

func TestMain(m *testing.M) {
	code := m.Run()
	if sharedBinDir != "" {
		os.RemoveAll(sharedBinDir)
	}
	os.Exit(code)
}

// runCLIDir is runCLI with an explicit working directory. Every subprocess in
// this suite runs in an empty temp dir so cwd-sensitive commands never touch
// the repo.
func runCLIDir(t *testing.T, bin, dir string, env []string, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()

	cmd := exec.CommandContext(ctx, bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if err != nil {
		exitErr, ok := err.(*exec.ExitError)
		if !ok {
			// Launch failure (binary missing, EMFILE, ctx timeout): fail loudly
			// instead of masquerading as exit 0.
			t.Fatalf("subprocess failed to run: %v", err)
		}
		exitCode = exitErr.ExitCode()
	}
	return outBuf.String(), errBuf.String(), exitCode
}

// contractEnv is apiEnv plus the update-check bypass (this suite spawns
// hundreds of processes; each would otherwise fire a background npm request).
func contractEnv(apiBaseURL string) []string {
	return append(apiEnv(apiBaseURL), "SHOPLAZZA_CLI_NO_UPDATE_CHECK=1")
}

// ── leaf enumeration ──────────────────────────────────────────────────────────

type cliLeaf struct {
	path      []string
	hasDryRun bool
	denied    bool
}

// notScannable reports whether c opted out of the blind scan at its
// definition site (interactive, long-running, or writes the local
// filesystem). The annotation covers the whole subtree via the walk below.
func notScannable(c *cobra.Command) bool {
	return c.Annotations[cmdutil.AnnotationNotScannable] == "true"
}

// collectLeaves walks the full tree and returns every runnable command.
// Runnable nodes that also have children are included as leaves themselves.
func collectLeaves(t *testing.T) []cliLeaf {
	t.Helper()
	root := cmd.NewRootCmd()
	var leaves []cliLeaf
	var walk func(c *cobra.Command, path []string, parentDenied bool)
	walk = func(c *cobra.Command, path []string, parentDenied bool) {
		for _, sub := range c.Commands() {
			subPath := append(append([]string{}, path...), sub.Name())
			subDenied := parentDenied || notScannable(sub)
			if sub.Runnable() {
				leaves = append(leaves, cliLeaf{
					path:      subPath,
					hasDryRun: sub.Flags().Lookup("dry-run") != nil,
					denied:    subDenied,
				})
			}
			walk(sub, subPath, subDenied)
		}
	}
	walk(root, nil, false)
	if len(leaves) == 0 {
		t.Fatal("command tree enumeration returned no leaves")
	}
	return leaves
}

// minNotScannable is the number of leaves known to carry the annotation today:
// auth login/logout, completion, update, themes init/package/serve,
// theme-extension serve, app dev, checkout-extension dev.
const minNotScannable = 10

// TestContractSmoke_NotScannableFloor guards the annotation set: dropping
// below the floor means a definition-site annotation was lost, re-exposing
// the blind scan to an interactive or long-running command.
func TestContractSmoke_NotScannableFloor(t *testing.T) {
	testenv.IsolateConfigDir(t)
	var denied []string
	for _, leaf := range collectLeaves(t) {
		if leaf.denied {
			denied = append(denied, strings.Join(leaf.path, " "))
		}
	}
	if len(denied) < minNotScannable {
		t.Fatalf("only %d leaves carry %s, want >= %d; current: %s",
			len(denied), cmdutil.AnnotationNotScannable, minNotScannable, strings.Join(denied, ", "))
	}
	t.Logf("not-scannable leaves (%d): %s", len(denied), strings.Join(denied, ", "))
}

// ── Tier 1: --help wiring check for every scannable leaf ─────────────────────

func TestContractSmoke_Help(t *testing.T) {
	testenv.IsolateConfigDir(t)
	bin := sharedBinary(t)
	leaves := collectLeaves(t)
	env := []string{"SHOPLAZZA_CLI_NO_UPDATE_CHECK=1"}

	skipped := 0
	for _, leaf := range leaves {
		if leaf.denied {
			skipped++
			t.Logf("skip (not-scannable): %s", strings.Join(leaf.path, " "))
			continue
		}
		t.Run(strings.Join(leaf.path, "/"), func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{}, leaf.path...), "--help")
			stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, args...)
			if code != output.ExitOK {
				t.Fatalf("--help exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stdout, "Usage:") {
				t.Fatalf("--help output missing Usage section\nstdout: %s", stdout)
			}
		})
	}
	t.Logf("help-scanned %d leaves, %d not-scannable", len(leaves)-skipped, skipped)
}

// ── Tier 2: dry-run output-contract check ─────────────────────────────────────

// Shape-only assertions (dry-run success is a bare preview; cobra errors are
// plain text); rationale in the design doc.
func TestContractSmoke_DryRun(t *testing.T) {
	testenv.IsolateConfigDir(t)
	bin := sharedBinary(t)
	leaves := collectLeaves(t)

	var mockHits atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mockHits.Add(1)
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	// t.Cleanup, not defer: parallel subtests run after this function returns.
	t.Cleanup(srv.Close)
	t.Cleanup(func() {
		// Dry-run must never send the request; any mock hit is a violation.
		if n := mockHits.Load(); n > 0 {
			t.Errorf("dry-run leaves sent %d real HTTP request(s) to the mock", n)
		}
	})
	env := contractEnv(srv.URL)

	var previewed atomic.Int64
	t.Cleanup(func() {
		// Guard against the whole tier degrading into the error branch
		// (currently ~114 leaves reach the exit-0 preview path).
		if n := previewed.Load(); n < 25 {
			t.Errorf("only %d leaves exercised the dry-run preview path (exit 0); want >= 25", n)
		} else {
			t.Logf("dry-run preview exercised by %d leaves", n)
		}
	})

	for _, leaf := range leaves {
		if !leaf.hasDryRun || leaf.denied {
			continue
		}
		t.Run(strings.Join(leaf.path, "/"), func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{}, leaf.path...), "--dry-run")
			stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, args...)

			if code < output.ExitOK || code > output.ExitInternal {
				t.Errorf("exit code %d outside contract range %d..%d", code, output.ExitOK, output.ExitInternal)
			}
			// Go panics exit with code 2 (inside the range), so catch them
			// via stderr explicitly.
			if strings.Contains(stderr, "panic:") {
				t.Errorf("panic detected\nstderr: %s", stderr)
			}
			switch {
			case code == output.ExitOK:
				previewed.Add(1)
				if strings.TrimSpace(stdout) == "" {
					t.Errorf("exit 0 but stdout empty\nstderr: %s", stderr)
				} else {
					var v any
					if err := json.Unmarshal([]byte(stdout), &v); err != nil {
						t.Errorf("exit 0 but stdout is not valid JSON\nstdout: %s", stdout)
					} else if bad := findUppercaseKey(v); bad != "" {
						t.Errorf("stdout JSON key %q is not snake_case\nstdout: %s", bad, stdout)
					}
				}
			case strings.TrimSpace(stderr) == "":
				t.Errorf("exit %d but stderr empty (silent failure)\nstdout: %s", code, stdout)
			case strings.HasPrefix(strings.TrimSpace(stderr), "{"):
				// JSON-looking errors must be a well-formed ok=false envelope.
				var envlp map[string]any
				if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envlp); err != nil {
					t.Errorf("exit %d with malformed JSON stderr: %v\nstderr: %s", code, err, stderr)
				} else if ok, _ := envlp["ok"].(bool); ok {
					t.Errorf("exit %d but stderr envelope says ok=true\nstderr: %s", code, stderr)
				}
			}
			if t.Failed() {
				t.Logf("cmd: %s --dry-run\nstdout: %s\nstderr: %s", strings.Join(leaf.path, " "), stdout, stderr)
			}
		})
	}
}

// findUppercaseKey walks decoded JSON and returns the first object key with an
// uppercase letter — CLI-local output keys are snake_case by convention.
func findUppercaseKey(v any) string {
	switch x := v.(type) {
	case map[string]any:
		for k, child := range x {
			if strings.ContainsFunc(k, func(r rune) bool { return r >= 'A' && r <= 'Z' }) {
				return k
			}
			if bad := findUppercaseKey(child); bad != "" {
				return bad
			}
		}
	case []any:
		for _, child := range x {
			if bad := findUppercaseKey(child); bad != "" {
				return bad
			}
		}
	}
	return ""
}

// ── forced error: the JSON error envelope contract ────────────────────────────

// firstJSONObject decodes the first JSON object found in s (the envelope is
// multi-line indented JSON), tolerating stray text before or after it.
func firstJSONObject(s string) (map[string]any, bool) {
	start := strings.Index(s, "{")
	if start < 0 {
		return nil, false
	}
	dec := json.NewDecoder(strings.NewReader(s[start:]))
	var m map[string]any
	if err := dec.Decode(&m); err != nil {
		return nil, false
	}
	return m, true
}

func TestContractSmoke_ErrorEnvelope(t *testing.T) {
	testenv.IsolateConfigDir(t)
	bin := sharedBinary(t)
	env := contractEnv("http://127.0.0.1:1") // closed port forces a network error

	stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, "products", "list")
	if code == output.ExitOK {
		t.Fatalf("expected non-zero exit\nstdout: %s", stdout)
	}
	if code < output.ExitAPI || code > output.ExitInternal {
		t.Errorf("exit code %d outside contract range %d..%d", code, output.ExitAPI, output.ExitInternal)
	}
	envelope, found := firstJSONObject(stderr)
	if !found {
		t.Fatalf("no JSON envelope line on stderr\nstderr: %s", stderr)
	}
	if ok, _ := envelope["ok"].(bool); ok {
		t.Error("error envelope must carry ok=false")
	}
	if envelope["error"] == nil {
		t.Error("error envelope missing error field")
	}
}
