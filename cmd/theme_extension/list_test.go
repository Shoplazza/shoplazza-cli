package theme_extension

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestList_RunE_NoStoreDomain hits the resolveStore guard in newCmdList:
// with no --store-domain and empty f.Config.StoreDomain, it returns a
// validation error immediately.
func TestList_RunE_NoStoreDomain(t *testing.T) {
	cmd := newCmdList(&cmdutil.Factory{})
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Error("expected error when no store domain configured")
	}
}
