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

// charDevice opens os.DevNull — a character device on every supported platform
// (NUL on Windows). Note what it is NOT: a terminal. See TestIsTTY_DevNull in
// the internal test.
func charDevice(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// pipeFile returns an *os.File that is a pipe. Both ends stay open for the
// test's lifetime so Stat keeps working.
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

// IsTerminal answers "can I draw here", for which a character device is enough.
// It is NOT the gate's predicate — see the note on IsTTY.
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

// TestInteractive_ClosedWithoutARealTerminal is the assertion that matters for
// every non-human caller: pipes and character devices alike must leave the gate
// shut, whatever the environment says. Under `go test` there is no tty to hand,
// which is exactly the situation a CI job or an agent is in.
//
// The opposite direction — a terminal IS present and an escape hatch is set —
// needs the terminal answer injected and lives in the internal test.
func TestInteractive_ClosedWithoutARealTerminal(t *testing.T) {
	envCases := []struct {
		name string
		env  map[string]string
	}{
		{"neither set", nil},
		{"CI present but empty", map[string]string{envCI: ""}},
		{"CI set", map[string]string{envCI: "1"}},
		{"no-interactive present but empty", map[string]string{envNoInteractive: ""}},
		{"both present but empty", map[string]string{envNoInteractive: "", envCI: ""}},
		{"both set", map[string]string{envNoInteractive: "1", envCI: "true"}},
		{"CI=false still means CI", map[string]string{envCI: "false"}},
	}

	dev, pipe := charDevice(t), pipeFile(t)
	streamCases := []struct {
		name       string
		in, errOut *os.File
	}{
		{"both /dev/null", dev, dev},
		{"stdin a pipe", pipe, dev},
		{"stderr a pipe", dev, pipe},
		{"both pipes", pipe, pipe},
	}

	for _, ec := range envCases {
		for _, sc := range streamCases {
			if output.Interactive(sc.in, sc.errOut, envOf(ec.env)) {
				t.Errorf("Interactive(%s | %s) opened the gate; a prompt here waits forever", ec.name, sc.name)
			}
		}
	}
}

// The gate must read the environment before looking at the streams, and must
// read only the two documented hatches — an extra key here would be an
// undocumented way to change behaviour.
func TestInteractive_ConsultsExactlyTheTwoHatches(t *testing.T) {
	dev := charDevice(t)
	var asked []string
	env := func(k string) (string, bool) {
		asked = append(asked, k)
		return "", false
	}
	output.Interactive(dev, dev, env)

	if len(asked) != 2 {
		t.Fatalf("gate consulted %v, want exactly the two escape hatches", asked)
	}
	for _, want := range []string{envNoInteractive, envCI} {
		found := false
		for _, k := range asked {
			if k == want {
				found = true
			}
		}
		if !found {
			t.Errorf("gate never consulted %s", want)
		}
	}
}
