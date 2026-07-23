package jsbuild

import (
	"context"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// EnsureProjectDeps auto-installs the project's npm dependencies when they have
// never been installed: scaffolded extension projects declare runtime deps
// (shoplazza-extension-ui) the bundler must inline, but `checkout create` does
// not run `npm install`, so the first build would fail on an unresolved import.
//
// It is a no-op unless ALL of: <projectRoot>/package.json exists, no
// node_modules exists in projectRoot or any ancestor (mirroring Node module
// resolution, so a workspace-root install is honored when cwd is a subdir),
// and `npm` is on PATH (when npm is absent we proceed and let the build's
// unresolved-import validation hint guide the user instead).
func EnsureProjectDeps(ctx context.Context, projectRoot string) *output.ExitError {
	if st, err := os.Stat(filepath.Join(projectRoot, "package.json")); err != nil || st.IsDir() {
		return nil
	}
	if hasNodeModulesUpward(projectRoot) {
		return nil
	}
	npmPath, err := exec.LookPath("npm")
	if err != nil {
		return nil
	}

	fmt.Fprintf(os.Stderr, "node_modules not found — running `npm install` in %s ...\n", projectRoot)
	cmd := exec.CommandContext(ctx, npmPath, "install", "--no-audit", "--no-fund")
	cmd.Dir = projectRoot
	cmd.Stdout = os.Stderr // keep stdout clean for the build's JSON protocol
	cmd.Stderr = os.Stderr
	if err := cmd.Run(); err != nil {
		return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			fmt.Sprintf("automatic dependency install failed: %s", err.Error()),
			"run `npm install` in the project root manually, then retry")
	}
	return nil
}

// hasNodeModulesUpward reports whether dir or any ancestor contains a
// node_modules directory.
func hasNodeModulesUpward(dir string) bool {
	for {
		if st, err := os.Stat(filepath.Join(dir, "node_modules")); err == nil && st.IsDir() {
			return true
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			return false
		}
		dir = parent
	}
}
