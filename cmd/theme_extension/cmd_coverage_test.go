package theme_extension

import (
	"testing"

	"shoplazza-cli-v2/internal/cmdutil"
)

// TestBuild_PreRunE_Validations covers newCmdBuild's three pre-flight checks
// (all return before requireLogin, so no auth/network runs).
func TestBuild_PreRunE_Validations(t *testing.T) {
	f := &cmdutil.Factory{}

	// missing --version
	cmd := newCmdBuild(f)
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --version is missing")
	}

	// invalid --version format
	cmd = newCmdBuild(f)
	_ = cmd.Flags().Set("version", "not-semver")
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error for invalid --version format")
	}

	// valid version but missing --description
	cmd = newCmdBuild(f)
	_ = cmd.Flags().Set("version", "1.0.0")
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --description is missing")
	}
}

// TestBuild_RunE_NotATEProjectErrors drives newCmdBuild's RunE to its first step
// (te.ReadConfig) and asserts the "not a te project" error on a bare dir.
func TestBuild_RunE_NotATEProjectErrors(t *testing.T) {
	cmd := newCmdBuild(&cmdutil.Factory{})
	_ = cmd.Flags().Set("path", t.TempDir())
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected error when --path is not a te project")
	}
}

// TestServe_PreRunE_RequiresThemeID covers newCmdServe's --theme-id gate.
func TestServe_PreRunE_RequiresThemeID(t *testing.T) {
	cmd := newCmdServe(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --theme-id is missing")
	}
}

// TestServe_RunE_NotATEProjectErrors drives newCmdServe's RunE to te.ReadConfig.
func TestServe_RunE_NotATEProjectErrors(t *testing.T) {
	cmd := newCmdServe(&cmdutil.Factory{})
	_ = cmd.Flags().Set("path", t.TempDir())
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Fatal("expected error when --path is not a te project")
	}
}

// TestCreate_PreRunE covers both early-exit validation branches.
func TestCreate_PreRunE_RequiresName(t *testing.T) {
	cmd := newCmdCreate(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --name is missing")
	}
}

func TestCreate_PreRunE_RequiresValidType(t *testing.T) {
	cmd := newCmdCreate(&cmdutil.Factory{})
	_ = cmd.Flags().Set("name", "myext")
	_ = cmd.Flags().Set("type", "unsupported")
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error for unsupported --type")
	}
}

func TestCreate_PreRunE_PassesWithValidArgs(t *testing.T) {
	cmd := newCmdCreate(&cmdutil.Factory{})
	_ = cmd.Flags().Set("name", "myext")
	_ = cmd.Flags().Set("type", "basic")
	if err := cmd.PreRunE(cmd, nil); err != nil {
		t.Errorf("unexpected PreRunE error: %v", err)
	}
}

// TestConnect_PreRunE_MissingClientID hits the first guard in newCmdConnect.
func TestConnect_PreRunE_MissingClientID(t *testing.T) {
	cmd := newCmdConnect(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --client-id is missing")
	}
}

// TestDeploy_PreRunE_MissingVersionID hits the first guard in newCmdDeploy.
func TestDeploy_PreRunE_MissingVersionID(t *testing.T) {
	cmd := newCmdDeploy(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --version-id is missing")
	}
}

// TestRelease_PreRunE_MissingVersionID hits the first guard in newCmdRelease.
func TestRelease_PreRunE_MissingVersionID(t *testing.T) {
	cmd := newCmdRelease(&cmdutil.Factory{})
	if err := cmd.PreRunE(cmd, nil); err == nil {
		t.Error("expected error when --version-id is missing")
	}
}

// TestVersions_RunE_NoExtensionConfig drives the first RunE step in newCmdVersions:
// te.RequireExtensionID fails when no shoplazza.extension.toml exists in path.
func TestVersions_RunE_NoExtensionConfig(t *testing.T) {
	cmd := newCmdVersions(&cmdutil.Factory{})
	cmd.Flags().Set("path", t.TempDir())
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Error("expected error when no extension config in path")
	}
}

// TestList_RunE_NoStoreDomain hits the resolveStore guard in newCmdList:
// with no --store-domain and empty f.Config.StoreDomain, it returns a
// validation error immediately.
func TestList_RunE_NoStoreDomain(t *testing.T) {
	cmd := newCmdList(&cmdutil.Factory{})
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Error("expected error when no store domain configured")
	}
}
