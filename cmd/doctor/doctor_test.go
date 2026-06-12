package doctor

import "testing"

func TestNewCmdDoctor_Structure(t *testing.T) {
	cmd := NewCmdDoctor()
	if cmd.Use != "doctor" {
		t.Errorf("Use = %q, want doctor", cmd.Use)
	}
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "check" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'check' subcommand under doctor")
	}
}

func TestNewCmdCheck_RunEReturnsError(t *testing.T) {
	cmd := newCmdCheck()
	if cmd.Use != "check" {
		t.Errorf("Use = %q, want check", cmd.Use)
	}
	err := cmd.RunE(cmd, nil)
	if err == nil {
		t.Error("expected non-nil error from check RunE (not yet implemented)")
	}
}
