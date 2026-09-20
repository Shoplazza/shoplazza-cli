package cmd

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// TestDefaultOutputFormat: SHOPLAZZA_CLI_FORMAT sets the --format default when
// valid; unset or invalid falls back to json.
func TestDefaultOutputFormat(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "")
	if got := defaultOutputFormat(); got != output.FormatJSON {
		t.Errorf("unset = %q, want json", got)
	}
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "pretty")
	if got := defaultOutputFormat(); got != output.FormatPretty {
		t.Errorf("pretty env = %q, want pretty", got)
	}
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "bogus")
	if got := defaultOutputFormat(); got != output.FormatJSON {
		t.Errorf("invalid env = %q, want json fallback", got)
	}
}

// TestWantsNoInput: --no-input is detected before the -- terminator.
func TestWantsNoInput(t *testing.T) {
	if !wantsNoInput([]string{"themes", "env", "add", "--no-input"}) {
		t.Error("should detect --no-input")
	}
	if !wantsNoInput([]string{"--no-input=true"}) {
		t.Error("should detect --no-input=true")
	}
	if wantsNoInput([]string{"themes", "list"}) {
		t.Error("absent → false")
	}
	if wantsNoInput([]string{"--", "--no-input"}) {
		t.Error("after -- it is an operand, not a flag")
	}
	// All truthy pflag bool forms enable it.
	for _, tv := range []string{"--no-input=1", "--no-input=t", "--no-input=TRUE", "--no-input=True"} {
		if !wantsNoInput([]string{tv}) {
			t.Errorf("%q should enable no-input", tv)
		}
	}
	// Falsy forms do not.
	for _, fv := range []string{"--no-input=false", "--no-input=0", "--no-input=F"} {
		if wantsNoInput([]string{fv}) {
			t.Errorf("%q must NOT enable no-input", fv)
		}
	}
}

// TestAutoPretty: pretty only when stdout is a terminal and no automation signal.
func TestAutoPretty(t *testing.T) {
	none := func(string) (string, bool) { return "", false }
	if !autoPretty(true, none, nil) {
		t.Error("terminal + no signals → pretty")
	}
	if autoPretty(false, none, nil) {
		t.Error("non-terminal → json")
	}
	// An empty CI value must not gate (treated as unset).
	ciEmpty := func(k string) (string, bool) { return "", k == "CI" }
	if !autoPretty(true, ciEmpty, nil) {
		t.Error("empty CI must not gate → pretty")
	}
	ciSet := func(k string) (string, bool) {
		if k == "CI" {
			return "1", true
		}
		return "", false
	}
	if autoPretty(true, ciSet, nil) {
		t.Error("CI=1 → json even on a terminal")
	}
	niSet := func(k string) (string, bool) {
		if k == "SHOPLAZZA_CLI_NO_INTERACTIVE" {
			return "1", true
		}
		return "", false
	}
	if autoPretty(true, niSet, nil) {
		t.Error("SHOPLAZZA_CLI_NO_INTERACTIVE=1 → json")
	}
	if autoPretty(true, none, []string{"--no-input"}) {
		t.Error("--no-input → json")
	}
}
