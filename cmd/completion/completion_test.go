package completion

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func TestNewCmdCompletion_Structure(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdCompletion(f)
	if cmd.Use != "completion <shell>" {
		t.Errorf("Use = %q, want 'completion <shell>'", cmd.Use)
	}
	validArgs := map[string]bool{}
	for _, a := range cmd.ValidArgs {
		validArgs[a] = true
	}
	for _, shell := range []string{"bash", "zsh", "fish", "powershell"} {
		if !validArgs[shell] {
			t.Errorf("expected %q in ValidArgs", shell)
		}
	}
}
