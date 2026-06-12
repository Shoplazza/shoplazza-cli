package cmdutil

import (
	"testing"

	"github.com/spf13/cobra"
)

func newCmdWithFlags(t *testing.T) *cobra.Command {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.Flags().String("format", "", "")
	cmd.Flags().Bool("dry-run", false, "")
	cmd.Flags().String("jq", "", "")
	return cmd
}

// ── GetFormat ─────────────────────────────────────────────────────────────────

func TestGetFormat_DefaultsToJSON(t *testing.T) {
	cmd := newCmdWithFlags(t)
	if got := GetFormat(cmd); got != "json" {
		t.Errorf("GetFormat (no flag) = %q, want json", got)
	}
}

func TestGetFormat_ReturnsSetValue(t *testing.T) {
	cmd := newCmdWithFlags(t)
	_ = cmd.Flags().Set("format", "pretty")
	if got := GetFormat(cmd); got != "pretty" {
		t.Errorf("GetFormat = %q, want pretty", got)
	}
}

func TestGetFormat_EmptyStringFallsBackToJSON(t *testing.T) {
	cmd := newCmdWithFlags(t)
	_ = cmd.Flags().Set("format", "")
	if got := GetFormat(cmd); got != "json" {
		t.Errorf("GetFormat (empty) = %q, want json", got)
	}
}

// ── IsDryRun ──────────────────────────────────────────────────────────────────

func TestIsDryRun_FalseByDefault(t *testing.T) {
	cmd := newCmdWithFlags(t)
	if IsDryRun(cmd) {
		t.Error("IsDryRun should be false by default")
	}
}

func TestIsDryRun_TrueWhenSet(t *testing.T) {
	cmd := newCmdWithFlags(t)
	_ = cmd.Flags().Set("dry-run", "true")
	if !IsDryRun(cmd) {
		t.Error("IsDryRun should be true when --dry-run is set")
	}
}

// ── GetJQ ─────────────────────────────────────────────────────────────────────

func TestGetJQ_EmptyByDefault(t *testing.T) {
	cmd := newCmdWithFlags(t)
	if got := GetJQ(cmd); got != "" {
		t.Errorf("GetJQ (no flag) = %q, want empty", got)
	}
}

func TestGetJQ_ReturnsSetExpression(t *testing.T) {
	cmd := newCmdWithFlags(t)
	_ = cmd.Flags().Set("jq", ".data.products[].id")
	if got := GetJQ(cmd); got != ".data.products[].id" {
		t.Errorf("GetJQ = %q, want .data.products[].id", got)
	}
}
