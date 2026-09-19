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
}
