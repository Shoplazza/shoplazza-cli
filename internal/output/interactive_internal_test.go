package output

import (
	"os"
	"testing"
)

// TestInteractive_EnvBeatsATerminal covers the direction that hangs builds: a
// real terminal IS present (an agent harness handing its child a pty) and an
// escape hatch is set. go test has no tty, so the terminal answer is injected.
func TestInteractive_EnvBeatsATerminal(t *testing.T) {
	yes := func() bool { return true }
	no := func() bool { return false }
	env := func(m map[string]string) func(string) (string, bool) {
		return func(k string) (string, bool) { v, ok := m[k]; return v, ok }
	}
	for _, c := range []struct {
		name string
		vars map[string]string
		tty  func() bool
		want bool
	}{
		{"terminal, nothing set", nil, yes, true},
		{"terminal, NO_INTERACTIVE=1", map[string]string{envNoInteractive: "1"}, yes, false},
		{"terminal, NO_INTERACTIVE=anything", map[string]string{envNoInteractive: "please"}, yes, false},
		{"terminal, CI=true", map[string]string{envCI: "true"}, yes, false},
		{"terminal, CI=1", map[string]string{envCI: "1"}, yes, false},
		{"terminal, CI=yes", map[string]string{envCI: "yes"}, yes, false},
		{"terminal, CI=false still closes", map[string]string{envCI: "false"}, yes, false},
		{"terminal, CI present but empty stays open", map[string]string{envCI: ""}, yes, true},
		{"no terminal, nothing set", nil, no, false},
		{"no terminal, CI=1", map[string]string{envCI: "1"}, no, false},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := interactive(env(c.vars), c.tty); got != c.want {
				t.Errorf("interactive() = %v, want %v", got, c.want)
			}
		})
	}
}

// TestIsTTY_DevNullIsNotATerminal is a regression test with a measured failure
// behind it. IsTerminal (os.ModeCharDevice) says yes to /dev/null, so
// `app init < /dev/null 2>/dev/null` — an ordinary CI and agent shape — used to
// open a prompt against the void and hang until killed.
func TestIsTTY_DevNullIsNotATerminal(t *testing.T) {
	f, err := os.OpenFile(os.DevNull, os.O_RDWR, 0)
	if err != nil {
		t.Fatalf("open %s: %v", os.DevNull, err)
	}
	defer f.Close()

	if !IsTerminal(f) {
		t.Fatalf("%s is expected to be a character device; the point of this test is that that is not enough", os.DevNull)
	}
	if IsTTY(f) {
		t.Errorf("IsTTY(%s) = true: a prompt opened here waits forever", os.DevNull)
	}
}

// TestIsTTY_PipeAndNil covers the other non-terminals the gate can be handed.
func TestIsTTY_PipeAndNil(t *testing.T) {
	r, w, err := os.Pipe()
	if err != nil {
		t.Fatalf("os.Pipe: %v", err)
	}
	defer func() { r.Close(); w.Close() }()
	if IsTTY(r) {
		t.Error("IsTTY(pipe) = true")
	}
	if IsTTY(nil) {
		t.Error("IsTTY(nil) = true")
	}
}

// envNoInteractive / envCI mirror interactiveOffEnv so a typo in the
// implementation surfaces here rather than as a dead branch.
const (
	envNoInteractive = "SHOPLAZZA_CLI_NO_INTERACTIVE"
	envCI            = "CI"
)

func TestInteractiveOffEnv_NamesMatch(t *testing.T) {
	want := map[string]bool{envNoInteractive: true, envCI: true}
	if len(interactiveOffEnv) != len(want) {
		t.Fatalf("interactiveOffEnv = %v", interactiveOffEnv)
	}
	for _, k := range interactiveOffEnv {
		if !want[k] {
			t.Errorf("unexpected escape hatch %q", k)
		}
	}
}
