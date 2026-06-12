package jsbuild

import (
	"os"
	"path/filepath"
	"testing"

	"shoplazza-cli-v2/internal/build"
)

func TestDerivePkgRoot(t *testing.T) {
	// npm global layout: <pkgRoot>/bin/shoplazza → pkgRoot is two dirs up.
	exe := filepath.FromSlash("/usr/lib/node_modules/shoplazza-cli/bin/shoplazza")
	got := derivePkgRoot(exe)
	want := filepath.FromSlash("/usr/lib/node_modules/shoplazza-cli")
	if got != want {
		t.Fatalf("derivePkgRoot = %q, want %q", got, want)
	}
}

func TestPkgRoot_EnvOverride(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_PKG_ROOT", filepath.FromSlash("/dev/repo/root"))
	got, err := PkgRoot()
	if err != nil {
		t.Fatalf("PkgRoot: %v", err)
	}
	if got != filepath.FromSlash("/dev/repo/root") {
		t.Fatalf("PkgRoot = %q, want /dev/repo/root", got)
	}
}

func setDevPkgRoot(t *testing.T, root string) {
	t.Helper()
	prev := build.DevPkgRoot
	build.DevPkgRoot = root
	t.Cleanup(func() { build.DevPkgRoot = prev })
}

func TestPkgRoot_BakedDevRoot(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_PKG_ROOT", "")
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "jsbuild"), 0o755); err != nil {
		t.Fatal(err)
	}
	setDevPkgRoot(t, repo)

	got, err := PkgRoot()
	if err != nil {
		t.Fatalf("PkgRoot: %v", err)
	}
	if got != repo {
		t.Fatalf("PkgRoot = %q, want baked dev root %q", got, repo)
	}
}

func TestPkgRoot_BakedDevRootStale_FallsBackToExe(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_PKG_ROOT", "")
	setDevPkgRoot(t, filepath.Join(t.TempDir(), "moved-away")) // no scripts/jsbuild here

	got, err := PkgRoot()
	if err != nil {
		t.Fatalf("PkgRoot: %v", err)
	}
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	if want := derivePkgRoot(exe); got != want {
		t.Fatalf("PkgRoot = %q, want exe-derived %q", got, want)
	}
}

func TestPkgRoot_EnvBeatsBakedDevRoot(t *testing.T) {
	repo := t.TempDir()
	if err := os.MkdirAll(filepath.Join(repo, "scripts", "jsbuild"), 0o755); err != nil {
		t.Fatal(err)
	}
	setDevPkgRoot(t, repo)
	t.Setenv("SHOPLAZZA_CLI_PKG_ROOT", filepath.FromSlash("/env/wins"))

	got, err := PkgRoot()
	if err != nil {
		t.Fatalf("PkgRoot: %v", err)
	}
	if got != filepath.FromSlash("/env/wins") {
		t.Fatalf("PkgRoot = %q, want /env/wins", got)
	}
}

func TestBuildEntryPath(t *testing.T) {
	root := filepath.FromSlash("/pkg/root")
	got := BuildEntryPath(root)
	want := filepath.Join(root, "scripts", "jsbuild", "index.js")
	if got != want {
		t.Errorf("BuildEntryPath = %q, want %q", got, want)
	}
}

func TestDevEntryPath(t *testing.T) {
	root := filepath.FromSlash("/pkg/root")
	got := DevEntryPath(root)
	want := filepath.Join(root, "scripts", "jsbuild", "dev", "index.js")
	if got != want {
		t.Errorf("DevEntryPath = %q, want %q", got, want)
	}
}

func TestNodePath_ReturnsNonEmpty(t *testing.T) {
	p, _ := NodePath()
	// If node is installed, path is non-empty. If not, skip — we only need to
	// cover the function itself, the error branch is a CI/env concern.
	if p == "" {
		t.Skip("node not on PATH in this environment — error branch, not a test failure")
	}
	if !filepath.IsAbs(p) {
		t.Errorf("NodePath = %q, want absolute path", p)
	}
}
