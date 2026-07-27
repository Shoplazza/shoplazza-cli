package appcmd

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// TestDeploy_Flags asserts newCmdDeploy registers its flags and the login gate.
func TestDeploy_Flags(t *testing.T) {
	cmd := newCmdDeploy(&cmdutil.Factory{})
	for _, name := range []string{"path", "debug"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("missing flag --%s", name)
		}
	}
	// --store-domain was removed: deploy always targets the current store.
	if cmd.Flags().Lookup("store-domain") != nil {
		t.Error("flag --store-domain should have been removed (deploy targets the current store)")
	}
	if cmd.PreRunE == nil {
		t.Error("expected PreRunE (requireLogin) to be set")
	}
}

// TestDeploy_RunE_NotAProjectErrors drives RunE to its first step (openProject)
// and asserts it errors when --path is not an app project. No auth/network runs.
func TestDeploy_RunE_NotAProjectErrors(t *testing.T) {
	cmd := newCmdDeploy(&cmdutil.Factory{})
	if err := cmd.Flags().Set("path", t.TempDir()); err != nil {
		t.Fatal(err)
	}
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected an error when --path is not an app project")
	}
}
