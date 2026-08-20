package output_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// The two escape hatches the gate honors. Named here so a typo in the
// implementation shows up as a failing test rather than a dead branch.
const (
	envNoInteractive = "SHOPLAZZA_CLI_NO_INTERACTIVE"
	envCI            = "CI"
)

// charDevice opens os.DevNull — the one character device present on every
// supported platform (NUL on Windows) — so "is a terminal" can be exercised
// without a real one.
func charDevice(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// pipeFile returns an *os.File that is a pipe, not a character device. Both ends
// stay open for the test's lifetime so Stat keeps working.
func pipeFile(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r
}

// envOf builds the injected lookup: only keys present in m are "set", and a
// present key may hold an empty value — the distinction the gate turns on.
func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

func TestIsTerminal(t *testing.T) {
	cases := []struct {
		name string
		v    any
		want bool
	}{
		{"character device", charDevice(t), true},
		{"pipe", pipeFile(t), false},
		{"bytes.Buffer exposes no Stat", &bytes.Buffer{}, false},
		{"nil", nil, false},
	}
	for _, c := range cases {
		if got := output.IsTerminal(c.v); got != c.want {
			t.Errorf("IsTerminal(%s) = %v, want %v", c.name, got, c.want)
		}
	}
}

// The gate is the product of two independent dimensions, so the test crosses
// them: each env var present-and-non-empty / present-but-empty / absent, against
// stdin and stderr each being a character device or not.
func TestInteractive_Gate(t *testing.T) {
	envCases := []struct {
		name    string
		env     map[string]string
		blocked bool
	}{
		{"neither set", nil, false},
		{"no-interactive absent, CI empty", map[string]string{envCI: ""}, false},
		{"no-interactive absent, CI set", map[string]string{envCI: "1"}, true},
		{"no-interactive empty, CI absent", map[string]string{envNoInteractive: ""}, false},
		{"both empty", map[string]string{envNoInteractive: "", envCI: ""}, false},
		{"no-interactive empty, CI set", map[string]string{envNoInteractive: "", envCI: "true"}, true},
		{"no-interactive set, CI absent", map[string]string{envNoInteractive: "1"}, true},
		{"no-interactive set, CI empty", map[string]string{envNoInteractive: "yes", envCI: ""}, true},
		{"both set", map[string]string{envNoInteractive: "1", envCI: "true"}, true},
		// Presence is what counts — the value is never compared, so even a
		// "falsy" one closes the gate.
		{"CI=false still means CI", map[string]string{envCI: "false"}, true},
		{"CI=0 still means CI", map[string]string{envCI: "0"}, true},
	}

	dev, pipe := charDevice(t), pipeFile(t)
	ttyCases := []struct {
		name   string
		in     *os.File
		errOut *os.File
		isTTY  bool
	}{
		{"stdin and stderr are character devices", dev, dev, true},
		{"stdin is not a character device", pipe, dev, false},
		{"stderr is not a character device", dev, pipe, false},
		{"neither is a character device", pipe, pipe, false},
	}

	for _, ec := range envCases {
		for _, tc := range ttyCases {
			want := !ec.blocked && tc.isTTY
			got := output.Interactive(tc.in, tc.errOut, envOf(ec.env))
			if got != want {
				t.Errorf("Interactive(%s | %s) = %v, want %v", ec.name, tc.name, got, want)
			}
		}
	}
}

// stdout is deliberately excluded from the gate: `shoplazza ... | jq` pipes
// stdout away while the prompt is still drawn on stderr. Under `go test` stdout
// is captured, so this asserts the gate opens regardless.
func TestInteractive_StdoutNotConsulted(t *testing.T) {
	if output.IsTerminal(os.Stdout) {
		t.Skip("stdout is a terminal here, nothing to prove")
	}
	dev := charDevice(t)
	if !output.Interactive(dev, dev, envOf(nil)) {
		t.Error("Interactive must ignore a non-terminal stdout")
	}
}

// A nil env lookup would be a programming error, but the gate reads the
// environment before anything else — guard the contract that callers must pass
// one (os.LookupEnv in production).
func TestInteractive_UsesInjectedEnvOnly(t *testing.T) {
	dev := charDevice(t)
	var asked []string
	env := func(k string) (string, bool) {
		asked = append(asked, k)
		return "", false
	}
	if !output.Interactive(dev, dev, env) {
		t.Fatal("gate must be open when the injected env sets nothing")
	}
	if len(asked) != 2 {
		t.Errorf("gate consulted %v, want exactly the two escape hatches", asked)
	}
	for _, want := range []string{envNoInteractive, envCI} {
		found := false
		for _, k := range asked {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("gate never consulted %s (asked %v)", want, asked)
		}
	}
}

// Cancel needs its own code: ExitAPI is already 1, so reusing it would make a
// user's Ctrl-C indistinguishable from a failed request.
func TestExitCanceled(t *testing.T) {
	if output.ExitCanceled != 130 {
		t.Errorf("ExitCanceled = %d, want 130 (128+SIGINT)", output.ExitCanceled)
	}
	if output.ExitCanceled == output.ExitAPI {
		t.Error("ExitCanceled must differ from ExitAPI")
	}
}
