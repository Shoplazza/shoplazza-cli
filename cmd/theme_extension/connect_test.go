package theme_extension

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestConnect_PreRunE_MissingClientID hits the first guard in newCmdConnect.
func TestConnect_PreRunE_MissingClientID(t *testing.T) {
	cmd := newCmdConnect(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --client-id is missing")
	}
}
