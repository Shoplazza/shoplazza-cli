package output

import "testing"

// alwaysTTY stands in for "stdin and stderr are both real terminals".
func alwaysTTY() bool { return true }

// noEnv is the environment with neither escape hatch set.
func noEnv(string) (string, bool) { return "", false }

// TestInteractive_RefusesATerminalTooNarrowToDraw covers the width half of the
// gate. A prompt on a 0-3 column terminal does not degrade, it panics inside
// bubbles' textinput, so those widths must count as no terminal at all.
func TestInteractive_RefusesATerminalTooNarrowToDraw(t *testing.T) {
	for _, tc := range []struct {
		name string
		cols int
		want bool
	}{
		{"no size reported", 0, false},
		{"widest width that panics", 3, false},
		{"one short of the minimum", minPromptCols - 1, false},
		{"exactly the minimum", minPromptCols, true},
		{"an ordinary terminal", 80, true},
	} {
		t.Run(tc.name, func(t *testing.T) {
			got := interactive(noEnv, alwaysTTY, func() int { return tc.cols })
			if got != tc.want {
				t.Errorf("interactive(cols=%d) = %v, want %v", tc.cols, got, tc.want)
			}
		})
	}
}

// TestInteractive_WidthNeverRescuesAClosedGate asserts the width is an extra
// condition, not an override: no terminal, or an escape hatch set, still wins.
func TestInteractive_WidthNeverRescuesAClosedGate(t *testing.T) {
	wide := func() int { return 200 }
	if interactive(noEnv, func() bool { return false }, wide) {
		t.Error("a wide window that is not a terminal opened the gate")
	}
	for _, k := range interactiveOffEnv {
		env := func(name string) (string, bool) {
			if name == k {
				return "1", true
			}
			return "", false
		}
		if interactive(env, alwaysTTY, wide) {
			t.Errorf("%s did not close the gate on a wide terminal", k)
		}
	}
}

// TestInteractive_EscapeHatchShortCircuitsTheSizeRead asserts the size is not
// read once an escape hatch is set: the ioctl is pointless there.
func TestInteractive_EscapeHatchShortCircuitsTheSizeRead(t *testing.T) {
	reads := 0
	env := func(string) (string, bool) { return "1", true }
	interactive(env, alwaysTTY, func() int { reads++; return 80 })
	if reads != 0 {
		t.Errorf("size read %d times behind an escape hatch, want 0", reads)
	}
}
