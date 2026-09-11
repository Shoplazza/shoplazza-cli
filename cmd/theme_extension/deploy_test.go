package theme_extension

import (
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// TestDeploy_PreRunE_MissingVersionID hits the first guard in newCmdDeploy.
func TestDeploy_PreRunE_MissingVersionID(t *testing.T) {
	cmd := newCmdDeploy(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --version-id is missing")
	}
}

// TestDeploy_PreRunE_CorruptConfigIsMalformed: same contract on the
// RequireExtensionID path — no "register first" hint for a corrupt file.
func TestDeploy_PreRunE_CorruptConfigIsMalformed(t *testing.T) {
	root := t.TempDir()
	writeCorruptConfig(t, root)
	cmd := newCmdDeploy(&cmdutil.Factory{})
	_ = cmd.Flags().Set("version", "1.0.0")
	_ = cmd.Flags().Set("path", root)
	err := cmd.PreRunE(cmd, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
	if !strings.Contains(ee.Error(), "malformed") || strings.Contains(ee.Error(), "register first") {
		t.Fatalf("expected malformed message without the register hint, got %q", ee.Error())
	}
}
