// Package tests: CI regression for the interactive layer's non-interactive
// contract. Runs the built binary with no TTY and stdin closed (exec.Command
// with no Stdin), so it is always in agent mode. Every command the interactive
// work rewired must, non-interactively, return a structured result fast — never
// prompt, never hang. This is the L2 self-test (scripts/selftest_interaction.sh)
// pinned into CI; see docs/M3_INTERACTION_TEST_PLAN.md.
package tests_test

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestInteraction_NonInteractiveContract(t *testing.T) {
	testenv.IsolateConfigDir(t) // hermetic: no dependence on the machine's real creds
	bin := buildBinary(t)

	// Any network goes here, never to a real API; destructive ops that reach
	// execute return against this instead of a live host.
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"code": "Success", "data": map[string]any{"extensions": []any{}}})
	}))
	defer srv.Close()
	env := apiEnv(srv.URL)

	// Missing required flag / dry-run / json: a specific substring must appear,
	// and the command must not hang.
	substr := []struct {
		name, want string
		args       []string
	}{
		{"shortcut fill: products +create", "required flag(s) not set", []string{"products", "+create"}},
		{"shortcut fill: products +publish", "--id", []string{"products", "+publish"}},
		{"shortcut fill: products +unpublish", "--id", []string{"products", "+unpublish"}},
		{"shortcut fill: products +tag", "--id", []string{"products", "+tag"}},
		{"shortcut fill: orders +refund", "--order-id", []string{"orders", "+refund", "--amount", "5"}},
		{"shortcut fill: orders +ship", "--order-id", []string{"orders", "+ship", "--tracking", "X"}},
		{"shortcut fill: orders +update-tracking", "--order-id", []string{"orders", "+update-tracking", "--tracking", "X"}},
		{"shortcut fill: themes push", "--theme-id", []string{"themes", "push"}},
		{"shortcut fill: themes pull", "--theme-id", []string{"themes", "pull"}},
		{"cmd fill: app function compile", "--name", []string{"app", "function", "compile"}},
		{"cmd fill: checkout deploy", "--extension-id", []string{"checkout-extension", "deploy"}},
		{"cmd fill: checkout preview", "--extension-id", []string{"checkout-extension", "preview"}},
		{"cmd fill: checkout undeploy", "--extension-id", []string{"checkout-extension", "undeploy"}},
		{"cmd fill: checkout push", "--name", []string{"checkout-extension", "push"}},
		{"cmd fill: checkout init", "--name", []string{"checkout-extension", "init"}},
		{"cmd fill: checkout extension add", "--name", []string{"checkout-extension", "create"}},
		{"dynamic dry-run: webhook delete", "dry_run", []string{"webhook", "delete", "--params", `{"id":"1"}`, "--dry-run"}},
		{"dynamic dry-run: orders cancel", "dry_run", []string{"orders", "cancel", "--params", `{"order_id":"1"}`, "--dry-run"}},
		{"cmd dry-run: checkout deploy", "dry_run", []string{"checkout-extension", "deploy", "--extension-id", "E1", "--version", "1.0", "--dry-run"}},
		{"json fill: products +create", "validation", []string{"products", "+create", "--format", "json"}},
	}
	for _, c := range substr {
		t.Run(c.name, func(t *testing.T) {
			out := runFast(t, bin, env, c.args...)
			if !strings.Contains(out, c.want) {
				t.Errorf("want %q in output; got: %s", c.want, oneLine(out))
			}
		})
	}

	// app extension create / function release are gated by requireLogin, which a
	// clean CI env cannot satisfy, so they reach a structured auth error rather
	// than the fill prompt (the ResolveFlags fill itself is unit-tested). What
	// still matters here is the contract: a structured error envelope, no prompt,
	// no hang.
	for _, c := range []struct {
		name string
		args []string
	}{
		{"cmd structured: app extension create", []string{"app", "extension", "create"}},
		{"cmd structured: app function release", []string{"app", "function", "release"}},
	} {
		t.Run(c.name, func(t *testing.T) {
			out := runFast(t, bin, env, c.args...)
			if !strings.Contains(out, `"error"`) || !strings.Contains(out, `"type"`) {
				t.Errorf("want a structured error envelope; got: %s", oneLine(out))
			}
		})
	}

	// Destructive ops: proceed without a prompt or hang → produce a terminal
	// result (the human-only confirm is skipped non-interactively).
	proceed := []struct {
		name string
		args []string
	}{
		{"dynamic DELETE: webhook delete", []string{"webhook", "delete", "--params", `{"id":"1"}`}},
		{"dynamic verb: orders cancel", []string{"orders", "cancel", "--params", `{"order_id":"1"}`}},
		{"shortcut: products +unpublish", []string{"products", "+unpublish", "--id", "1"}},
		{"cmd: profile remove", []string{"profile", "remove", "--name", "__selftest_absent__"}},
	}
	for _, c := range proceed {
		t.Run("destructive proceeds: "+c.name, func(t *testing.T) {
			if out := runFast(t, bin, env, c.args...); strings.TrimSpace(out) == "" {
				t.Errorf("no output — a destructive op must not stall on a prompt: %v", c.args)
			}
		})
	}
}

// runFast runs the binary and fails if it blocks (a non-interactive command must
// never wait on a prompt). runCLI already caps the child at 10s; the 5s guard
// turns a hang into a clear failure rather than a slow pass.
func runFast(t *testing.T, bin string, env []string, args ...string) string {
	t.Helper()
	start := time.Now()
	stdout, stderr, _ := runCLI(t, bin, env, args...)
	if d := time.Since(start); d > 5*time.Second {
		t.Errorf("took %s — a non-interactive command must not block", d)
	}
	return stdout + stderr
}

func oneLine(s string) string {
	s = strings.TrimSpace(s)
	if i := strings.IndexByte(s, '\n'); i >= 0 {
		return s[:i]
	}
	return s
}
