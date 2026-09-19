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
