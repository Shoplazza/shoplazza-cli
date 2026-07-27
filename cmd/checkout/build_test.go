package checkout_test

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	checkout "github.com/Shoplazza/shoplazza-cli/v2/cmd/checkout"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func TestResolveBuildTarget_FromIDFlag(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "demo")
	if err := os.MkdirAll(filepath.Join(extDir, "src"), 0o755); err != nil {
		t.Fatal(err)
	}
	id, path, projRoot, exitErr := checkout.ResolveBuildTarget(root, "demo")
	if exitErr != nil {
		t.Fatalf("unexpected: %v", exitErr)
	}
	if id != "demo" || path != extDir {
		t.Fatalf("got id=%q path=%q", id, path)
	}
	if projRoot != root {
		t.Fatalf("projectRoot = %q, want %q", projRoot, root)
	}
}

func TestResolveBuildTarget_NoIDOutsideExtension(t *testing.T) {
	root := t.TempDir()
	_, _, _, exitErr := checkout.ResolveBuildTarget(root, "")
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("expected type=validation, got %v", exitErr)
	}
}

// TestResolveBuildTarget_PathEscapingNameRejected: --name must be a plain
// directory name — "../../x" previously escaped ./extensions via Join+Clean.
func TestResolveBuildTarget_PathEscapingNameRejected(t *testing.T) {
	root := t.TempDir()
	_, _, _, exitErr := checkout.ResolveBuildTarget(root, "../../x")
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("path-escaping --name must be type=validation, got %v", exitErr)
	}
	if !strings.Contains(exitErr.Detail.Message, "plain name") {
		t.Errorf("must be rejected by the plain-name guard (not a stat miss), got %q", exitErr.Detail.Message)
	}
}

func TestResolveBuildTarget_NonexistentID(t *testing.T) {
	root := t.TempDir()
	_, _, _, exitErr := checkout.ResolveBuildTarget(root, "ghost")
	if exitErr == nil || exitErr.Detail.Type != output.TypeValidation {
		t.Fatalf("nonexistent --name must be type=validation, got %v", exitErr)
	}
}

func TestResolveBuildTarget_CwdInsideExtensionDir(t *testing.T) {
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "inside")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(extDir, "extension.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	id, _, projRoot, exitErr := checkout.ResolveBuildTarget(extDir, "")
	if exitErr != nil {
		t.Fatalf("unexpected: %v", exitErr)
	}
	if id != "inside" {
		t.Fatalf("got id=%q, want inside", id)
	}
	// The Node entry is <projectRoot>/extensions/<id>/src/index.js, so the
	// project root must be two levels above the extension dir — passing the
	// extension dir itself made the build look for extensions/<id>/extensions/<id>/.
	if projRoot != root {
		t.Fatalf("projectRoot = %q, want project root %q", projRoot, root)
	}
}

func TestResolveBuildTarget_CwdIsExtensionsDir(t *testing.T) {
	// Running inside extensions/ itself with --name: root is the parent.
	root := t.TempDir()
	extDir := filepath.Join(root, "extensions", "demo")
	if err := os.MkdirAll(extDir, 0o755); err != nil {
		t.Fatal(err)
	}
	id, _, projRoot, exitErr := checkout.ResolveBuildTarget(filepath.Join(root, "extensions"), "demo")
	if exitErr != nil {
		t.Fatalf("unexpected: %v", exitErr)
	}
	if id != "demo" || projRoot != root {
		t.Fatalf("got id=%q projectRoot=%q, want demo / %q", id, projRoot, root)
	}
}

func TestBuildCmd_MissingIDValidation(t *testing.T) {
	f := &cmdutil.Factory{IOStreams: cmdutil.IOStreams{}}
	cmd := checkout.NewCmdCheckout(f)
	cmd.SetArgs([]string{"build"})
	cmd.SetContext(context.Background())
	old, _ := os.Getwd()
	defer os.Chdir(old)
	os.Chdir(t.TempDir())
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected type=validation, got %v", err)
	}
}
