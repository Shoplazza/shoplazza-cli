package theme_extension

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestRelease_PreRunE_MissingVersionID hits the first guard in newCmdRelease.
func TestRelease_PreRunE_MissingVersionID(t *testing.T) {
	cmd := newCmdRelease(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --version-id is missing")
	}
}
