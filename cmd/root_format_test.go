package cmd

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
)

// TestDefaultOutputFormat verifies the --format default resolution:
// SHOPLAZZA_CLI_FORMAT > config.Format > "json", ignoring invalid values.
func TestDefaultOutputFormat(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "")

	if got := defaultOutputFormat(core.CliConfig{}); got != "json" {
		t.Errorf("empty config = %q, want json", got)
	}
	if got := defaultOutputFormat(core.CliConfig{Format: "pretty"}); got != "pretty" {
		t.Errorf("config pretty = %q, want pretty", got)
	}
	if got := defaultOutputFormat(core.CliConfig{Format: "bogus"}); got != "json" {
		t.Errorf("invalid config format = %q, want json fallback", got)
	}

	// Env var overrides config.
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "table")
	if got := defaultOutputFormat(core.CliConfig{Format: "pretty"}); got != "table" {
		t.Errorf("env override = %q, want table", got)
	}
	// An invalid env var is ignored, falling back to config.
	t.Setenv("SHOPLAZZA_CLI_FORMAT", "nonsense")
	if got := defaultOutputFormat(core.CliConfig{Format: "pretty"}); got != "pretty" {
		t.Errorf("invalid env = %q, want config pretty", got)
	}
}
