package theme_extension

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// TestCreate_NameValidation: --name is a path segment, not a path — traversal,
// nesting, absolute paths and hostile charsets must be rejected BEFORE any
// filesystem op (`--name ../escapee` used to scaffold outside the cwd).
func TestCreate_NameValidation(t *testing.T) {
	bad := []string{
		"../escapee", "..", "a/b", "/abs", `a\b`,
		"-leading-dash", ".hidden", "bad*char", "with space",
		"x123456789012345678901234567890123456789012345678901234567890123456789", // >64
	}
	for _, name := range bad {
		t.Run(name, func(t *testing.T) {
			cmd := newCmdCreate(&cmdutil.Factory{})
			_ = cmd.Flags().Set("name", name)
			_ = cmd.Flags().Set("type", "basic")
			err := cmd.PreRunE(cmd, nil)
			var ee *output.ExitError
			if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
				t.Fatalf("name %q: expected validation error, got %v", name, err)
			}
		})
	}
	for _, name := range []string{"myext", "My-Ext_2", "0day"} {
		cmd := newCmdCreate(&cmdutil.Factory{})
		_ = cmd.Flags().Set("name", name)
		_ = cmd.Flags().Set("type", "basic")
		if err := cmd.PreRunE(cmd, nil); err != nil {
			t.Fatalf("name %q should be accepted: %v", name, err)
		}
	}
}

// TestCreate_TraversalNeverTouchesParent: belt and braces on top of the regex —
// a full Execute with a traversal name must not materialize anything outside
// the cwd.
func TestCreate_TraversalNeverTouchesParent(t *testing.T) {
	parent := t.TempDir()
	work := filepath.Join(parent, "work")
	if err := os.Mkdir(work, 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(work)
	cmd := newCmdCreate(&cmdutil.Factory{})
	cmd.SetArgs([]string{"--name", "../escapee", "--type", "basic"})
	if err := cmd.Execute(); err == nil {
		t.Fatal("expected validation error for traversal name")
	}
	if _, err := os.Stat(filepath.Join(parent, "escapee")); !os.IsNotExist(err) {
		t.Fatalf("traversal name escaped the cwd: stat err=%v", err)
	}
}

// TestCreate_StatFailureIsInternal: a non-ENOENT stat (here: cwd without
// search permission) means "already exists" is unproven — internal error, no
// scaffold attempt.
func TestCreate_StatFailureIsInternal(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not deny access on Windows, so the stat can't fail")
	}
	if os.Geteuid() == 0 {
		t.Skip("permission checks don't bind for root")
	}
	dir := t.TempDir()
	t.Chdir(dir)
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(dir, 0o755) })

	cmd := newCmdCreate(&cmdutil.Factory{})
	cmd.SetArgs([]string{"--name", "myext", "--type", "basic"})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitInternal {
		t.Fatalf("expected internal error on non-ENOENT stat, got %v", err)
	}
}
