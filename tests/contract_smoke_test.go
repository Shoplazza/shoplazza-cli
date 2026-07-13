// Contract smoke suite: enumerates every runnable leaf command in-process via
// cmd.NewRootCmd(), then exercises each one through the real compiled binary.
// Tier 1 checks --help wiring; Tier 2 checks the output contract under
// --dry-run against a mock server. Design: docs/superpowers/specs/
// 2026-07-11-cli-contract-smoke-suite-design.md
package tests_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"shoplazza-cli-v2/cmd"

	"github.com/spf13/cobra"
)

// ── shared binary (built once for the whole package) ─────────────────────────

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
		if out, err := build.CombinedOutput(); err != nil {
			sharedBinErr = err
			sharedBinPath = string(out)
		}
	})
	if sharedBinErr != nil {
		t.Fatalf("build shared binary: %v\n%s", sharedBinErr, sharedBinPath)
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
	cmd := exec.Command(bin, args...)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), env...)
	var outBuf, errBuf strings.Builder
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf
	err := cmd.Run()
	exitCode = 0
	if exitErr, ok := err.(*exec.ExitError); ok {
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

// denylist holds command paths excluded from the blind scan. Entries are
// resolved through cobra (aliases work) and cover the whole subtree.
var denylist = []struct {
	prefix string
	reason string
}{
	{"auth login", "interactive: waits on browser OAuth callback"},
	{"auth logout", "mutates local keychain"},
	{"completion", "prints shell scripts, not envelopes"},
	{"update", "attempts binary self-update"},
	{"themes init", "writes local filesystem"},
	{"themes package", "writes local filesystem"},
	{"themes serve", "long-running watch process"},
	{"theme-extension serve", "long-running watch process"},
	{"app dev", "long-running local dev server"},
	{"checkout dev", "long-running local dev server"},
}

// deniedNodes resolves every denylist entry to its command node, failing the
// test when an entry no longer exists so the list can't rot silently.
func deniedNodes(t *testing.T, root *cobra.Command) map[*cobra.Command]bool {
	t.Helper()
	denied := make(map[*cobra.Command]bool, len(denylist))
	for _, d := range denylist {
		found, rest, err := root.Find(strings.Split(d.prefix, " "))
		if err != nil || found == nil || len(rest) != 0 {
			t.Fatalf("denylist entry %q does not resolve to a command (rest=%v err=%v)", d.prefix, rest, err)
		}
		denied[found] = true
	}
	return denied
}

// collectLeaves walks the full tree and returns every runnable command.
// Runnable nodes that also have children are included as leaves themselves.
func collectLeaves(t *testing.T) []cliLeaf {
	t.Helper()
	root := cmd.NewRootCmd()
	denied := deniedNodes(t, root)
	var leaves []cliLeaf
	var walk func(c *cobra.Command, path []string, parentDenied bool)
	walk = func(c *cobra.Command, path []string, parentDenied bool) {
		for _, sub := range c.Commands() {
			subPath := append(append([]string{}, path...), sub.Name())
			subDenied := parentDenied || denied[sub]
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

// TestContractSmoke_DenylistResolves keeps denylist rot a named failure even
// though collectLeaves also enforces it.
func TestContractSmoke_DenylistResolves(t *testing.T) {
	deniedNodes(t, cmd.NewRootCmd())
}

// ── Tier 1: --help wiring check for every non-denylisted leaf ─────────────────

func TestContractSmoke_Help(t *testing.T) {
	leaves := collectLeaves(t)
	bin := sharedBinary(t)
	env := []string{"SHOPLAZZA_CLI_NO_UPDATE_CHECK=1"}

	skipped := 0
	for _, leaf := range leaves {
		if leaf.denied {
			skipped++
			t.Logf("skip (denylist): %s", strings.Join(leaf.path, " "))
			continue
		}
		leaf := leaf
		t.Run(strings.Join(leaf.path, "/"), func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{}, leaf.path...), "--help")
			stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, args...)
			if code != 0 {
				t.Fatalf("--help exit %d\nstdout: %s\nstderr: %s", code, stdout, stderr)
			}
			if !strings.Contains(stdout, "Usage:") {
				t.Fatalf("--help output missing Usage section\nstdout: %s", stdout)
			}
		})
	}
	t.Logf("help-scanned %d leaves, %d denylisted", len(leaves)-skipped, skipped)
}

// ── Tier 2: dry-run output-contract check ─────────────────────────────────────
//
// Shape-only assertions; see the design doc for why envelopes are NOT asserted
// here (dry-run success prints a bare request preview; cobra flag errors print
// plain text).

func TestContractSmoke_DryRun(t *testing.T) {
	leaves := collectLeaves(t)
	bin := sharedBinary(t)

	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{}`))
	}))
	defer srv.Close()
	env := contractEnv(srv.URL)

	for _, leaf := range leaves {
		if !leaf.hasDryRun || leaf.denied {
			continue
		}
		leaf := leaf
		t.Run(strings.Join(leaf.path, "/"), func(t *testing.T) {
			t.Parallel()
			args := append(append([]string{}, leaf.path...), "--dry-run")
			stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, args...)

			if code < 0 || code > 5 {
				t.Errorf("exit code %d outside contract range 0..5", code)
			}
			// Go panics exit with code 2 (inside the range), so catch them
			// via stderr explicitly.
			if strings.Contains(stderr, "panic:") {
				t.Errorf("panic detected\nstderr: %s", stderr)
			}
			if code == 0 {
				if strings.TrimSpace(stdout) == "" {
					t.Errorf("exit 0 but stdout empty\nstderr: %s", stderr)
				} else if !json.Valid([]byte(stdout)) {
					t.Errorf("exit 0 but stdout is not valid JSON\nstdout: %s", stdout)
				}
			} else if strings.TrimSpace(stderr) == "" {
				t.Errorf("exit %d but stderr empty (silent failure)\nstdout: %s", code, stdout)
			}
			if t.Failed() {
				t.Logf("cmd: %s --dry-run\nstdout: %s\nstderr: %s", strings.Join(leaf.path, " "), stdout, stderr)
			}
		})
	}
}

// ── forced error: the JSON error envelope contract ────────────────────────────

func TestContractSmoke_ErrorEnvelope(t *testing.T) {
	bin := sharedBinary(t)
	env := contractEnv("http://127.0.0.1:1") // closed port forces a network error

	stdout, stderr, code := runCLIDir(t, bin, t.TempDir(), env, "products", "list")
	if code == 0 {
		t.Fatalf("expected non-zero exit\nstdout: %s", stdout)
	}
	if code < 1 || code > 5 {
		t.Errorf("exit code %d outside contract range 1..5", code)
	}
	var envelope map[string]any
	if err := json.Unmarshal([]byte(strings.TrimSpace(stderr)), &envelope); err != nil {
		t.Fatalf("stderr is not JSON: %v\nstderr: %s", err, stderr)
	}
	if ok, _ := envelope["ok"].(bool); ok {
		t.Error("error envelope must carry ok=false")
	}
	if envelope["error"] == nil {
		t.Error("error envelope missing error field")
	}
}
