package update

import "testing"

func TestNewCmdUpdate_Structure(t *testing.T) {
	cmd := NewCmdUpdate(nil)
	if cmd.Use != "update" {
		t.Errorf("Use = %q, want update", cmd.Use)
	}
	if cmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag")
	}
}
