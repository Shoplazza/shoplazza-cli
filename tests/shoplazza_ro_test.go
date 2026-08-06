// shoplazza_ro_test.go pins the read-only eval wrapper's one guarantee: no call
// reaches the real binary without --dry-run (or being pure introspection).
//
// The wrapper is maintainer tooling, but run_case.mjs tells the agent under test
// "nothing you run can ever change the store" while pointing it at a real store,
// so the claim has to be enforced, not commented. It previously was not: the
// schema allow-list keyed on "first token without a leading dash", which also
// matched a flag VALUE — `--profile schema products +publish --id X` executed
// for real.

package tests_test

import (
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// fakeBin echoes its argv instead of calling the API, so a leaked write is
// observable without one.
const fakeBin = "#!/usr/bin/env bash\necho \"RAN: $*\"\n"

func roWrapper(t *testing.T) (wrapper, stub string) {
	t.Helper()
	if runtime.GOOS == "windows" {
		t.Skip("bash wrapper is not used on Windows")
	}
	wrapper, err := filepath.Abs("../skills/shoplazza-skill-eval/bin/shoplazza-ro")
	if err != nil {
		t.Fatalf("resolve wrapper: %v", err)
	}
	if _, err := os.Stat(wrapper); err != nil {
		t.Skipf("wrapper not present: %v", err)
	}
	stub = filepath.Join(t.TempDir(), "fake-shoplazza")
	if err := os.WriteFile(stub, []byte(fakeBin), 0o755); err != nil {
		t.Fatalf("write stub: %v", err)
	}
	return wrapper, stub
}

func runRO(t *testing.T, wrapper, stub string, args ...string) (string, int) {
	t.Helper()
	cmd := exec.Command(wrapper, args...)
	cmd.Env = append(os.Environ(), "SHOPLAZZA_RO_BIN="+stub, "SHOPLAZZA_RO_PROFILE=")
	out, err := cmd.CombinedOutput()
	code := 0
	if err != nil {
		var ee *exec.ExitError
		if !errors.As(err, &ee) {
			t.Fatalf("run wrapper: %v", err)
		}
		code = ee.ExitCode()
	}
	return string(out), code
}

func TestShoplazzaRO_RefusesWritesWithoutDryRun(t *testing.T) {
	wrapper, stub := roWrapper(t)

	// Each of these must never reach the binary. The --profile/--format cases
	// are the flag-VALUE hole: a value of "schema" used to open the gate.
	for _, args := range [][]string{
		{"products", "+publish", "--id", "X"},
		{"--profile", "schema", "products", "+publish", "--id", "X"},
		{"--format", "schema", "products", "+publish", "--id", "X"},
		{"products", "+publish", "--id", "schema"},
		{"products", "+publish", "--id", "X", "--dry-run=false"},
		{"products", "+publish", "--id", "X", "--dry-run=F"},     // ParseBool accepts F
		{"products", "+publish", "--id", "X", "--", "--dry-run"}, // -- would shield our append
	} {
		out, code := runRO(t, wrapper, stub, args...)
		if strings.Contains(out, "RAN:") {
			t.Errorf("write reached the binary: %v\n%s", args, out)
		}
		if code != 3 {
			t.Errorf("exit = %d, want 3 (refused) for %v", code, args)
		}
	}
}

func TestShoplazzaRO_AllowsIntrospectionAndDryRun(t *testing.T) {
	wrapper, stub := roWrapper(t)

	for _, tc := range []struct {
		args      []string
		wantDry   bool // the appended real --dry-run
		wantInArg string
	}{
		{args: []string{"schema", "products.list", "--view", "request"}, wantDry: false, wantInArg: "schema products.list"},
		// Help calls carry the appended --dry-run too: pflag can eat "--help" as
		// another flag's VALUE (`--id --help`), in which case cobra never
		// short-circuits and the call is a live write.
		{args: []string{"products", "+publish", "--help"}, wantDry: true, wantInArg: "--help"},
		{args: []string{"products", "+tag", "--id", "--help", "--add", "x"}, wantDry: true, wantInArg: "+tag"},
		{args: []string{"products", "+publish", "--id", "X", "--dry-run"}, wantDry: true, wantInArg: "+publish"},
	} {
		out, code := runRO(t, wrapper, stub, tc.args...)
		if code != 0 || !strings.Contains(out, "RAN:") {
			t.Errorf("should have run: %v (exit %d)\n%s", tc.args, code, out)
			continue
		}
		if !strings.Contains(out, tc.wantInArg) {
			t.Errorf("argv lost %q for %v: %s", tc.wantInArg, tc.args, out)
		}
		// Anything that is not pure introspection carries an appended --dry-run.
		if got := strings.HasSuffix(strings.TrimSpace(out), "--dry-run"); got != tc.wantDry {
			t.Errorf("appended --dry-run = %v, want %v for %v: %s", got, tc.wantDry, tc.args, out)
		}
	}
}

// A pinned profile must lead: --profile is a global flag, so appending it after
// the subcommand args is fragile ordering the wrapper should not rely on.
func TestShoplazzaRO_PinnedProfileLeads(t *testing.T) {
	wrapper, stub := roWrapper(t)
	cmd := exec.Command(wrapper, "products", "+publish", "--id", "X", "--dry-run")
	cmd.Env = append(os.Environ(), "SHOPLAZZA_RO_BIN="+stub, "SHOPLAZZA_RO_PROFILE=dev")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("run: %v\n%s", err, out)
	}
	if !strings.Contains(string(out), "RAN: --profile dev products") {
		t.Errorf("pinned profile should lead the argv, got: %s", out)
	}
}
