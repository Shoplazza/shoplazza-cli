// Package jsbuild locates and drives the Node/Vite toolchain shipped inside the
// shoplazza-cli npm package (scripts/jsbuild/ + node_modules/).
package jsbuild

import (
	"os"
	"os/exec"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/internal/build"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// PkgRoot resolves the npm package root containing scripts/jsbuild/ and node_modules/.
// Order: ① $SHOPLAZZA_CLI_PKG_ROOT (dev/test override) ② build.DevPkgRoot
// (repo path baked in by `make build`, honored only while scripts/jsbuild still
// exists there) ③ derived from the running executable, whose npm layout is
// <pkgRoot>/bin/shoplazza.
func PkgRoot() (string, error) {
	if env := os.Getenv("SHOPLAZZA_CLI_PKG_ROOT"); env != "" {
		return env, nil
	}
	if root := build.DevPkgRoot; root != "" {
		if st, err := os.Stat(filepath.Join(root, "scripts", "jsbuild")); err == nil && st.IsDir() {
			return root, nil
		}
	}
	exe, err := os.Executable()
	if err != nil {
		return "", err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	return derivePkgRoot(exe), nil
}

// derivePkgRoot maps <pkgRoot>/bin/shoplazza → <pkgRoot> (two dirs up).
func derivePkgRoot(exePath string) string {
	return filepath.Dir(filepath.Dir(exePath))
}

// BuildEntryPath returns <pkgRoot>/scripts/jsbuild/index.js.
func BuildEntryPath(pkgRoot string) string {
	return filepath.Join(pkgRoot, "scripts", "jsbuild", "index.js")
}

// DevEntryPath returns <pkgRoot>/scripts/jsbuild/dev/index.js.
func DevEntryPath(pkgRoot string) string {
	return filepath.Join(pkgRoot, "scripts", "jsbuild", "dev", "index.js")
}

// NodePath returns the absolute path to the `node` binary, or a type=validation
// error with an install hint if Node is not on PATH.
func NodePath() (string, *output.ExitError) {
	p, err := exec.LookPath("node")
	if err != nil {
		return "", output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"Node.js is required to run 'checkout build'/'checkout dev' but was not found on PATH",
			"install Node.js >= 14.18.0 (https://nodejs.org) and ensure `node` is on your PATH")
	}
	return p, nil
}
