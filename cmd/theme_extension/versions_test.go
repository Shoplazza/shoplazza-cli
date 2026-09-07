package theme_extension

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestVersions_RunE_NoExtensionConfig drives the first RunE step in newCmdVersions:
// te.RequireExtensionID fails when no shoplazza.extension.toml exists in path.
func TestVersions_RunE_NoExtensionConfig(t *testing.T) {
	cmd := newCmdVersions(&cmdutil.Factory{})
	cmd.Flags().Set("path", t.TempDir())
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Error("expected error when no extension config in path")
	}
}
