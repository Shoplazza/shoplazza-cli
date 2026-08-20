package output_test

import (
	"bytes"
	"os"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// The two escape hatches the gate honors.
const (
	envNoInteractive = "SHOPLAZZA_CLI_NO_INTERACTIVE"
	envCI            = "CI"
)

// charDevice opens os.DevNull: a character device, but not a terminal.
func charDevice(t *testing.T) *os.File {
	t.Helper()
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	t.Cleanup(func() { f.Close() })
	return f
}

// pipeFile returns the read end of a pipe; both ends stay open until cleanup.
func pipeFile(t *testing.T) *os.File {
	t.Helper()
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	t.Cleanup(func() { r.Close(); w.Close() })
	return r
}

// envOf builds the injected lookup; only keys present in m are "set", and a
// present key may hold an empty value.
func envOf(m map[string]string) func(string) (string, bool) {
	return func(k string) (string, bool) {
		v, ok := m[k]
		return v, ok
	}
}

// TestIsTerminal covers character devices, pipes, writers with no Stat and nil.
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

// TestInteractive_ClosedWithoutARealTerminal asserts pipes and character devices
// alike leave the gate shut, whatever the environment says.
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

// TestInteractive_ConsultsExactlyTheTwoHatches asserts the gate reads those two
// keys and nothing else.
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
