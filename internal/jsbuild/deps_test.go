package jsbuild

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

// fakeNpm puts a stub `npm` on PATH that records its args and cwd to logPath,
// then runs extra shell commands (e.g. "mkdir node_modules" or "exit 1").
func fakeNpm(t *testing.T, logPath, extra string) {
	t.Helper()
	bin := t.TempDir()
	script := "#!/bin/sh\necho \"$@\" > " + logPath + "\npwd >> " + logPath + "\n" + extra + "\n"
	if err := os.WriteFile(filepath.Join(bin, "npm"), []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Setenv("PATH", bin)
}

// newProject creates a dir with a package.json, mimicking a scaffolded app root.
func newProject(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	if err := os.WriteFile(filepath.Join(root, "package.json"), []byte(`{"name":"p"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	return root
}

func TestEnsureProjectDeps_InstallsWhenNodeModulesMissing(t *testing.T) {
	root := newProject(t)
	log := filepath.Join(t.TempDir(), "npm.log")
	fakeNpm(t, log, "")

	if exitErr := EnsureProjectDeps(context.Background(), root); exitErr != nil {
		t.Fatalf("unexpected error: %v", exitErr)
	}
	rec, err := os.ReadFile(log)
	if err != nil {
		t.Fatalf("npm was not invoked: %v", err)
	}
	got := string(rec)
	if !strings.Contains(got, "install") {
		t.Errorf("npm args %q should contain 'install'", got)
	}
	if resolved, _ := filepath.EvalSymlinks(root); !strings.Contains(got, resolved) && !strings.Contains(got, root) {
		t.Errorf("npm should run in project root %q, got %q", root, got)
	}
}

func TestEnsureProjectDeps_SkipsWhenNodeModulesPresent(t *testing.T) {
	root := newProject(t)
	if err := os.Mkdir(filepath.Join(root, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "npm.log")
	fakeNpm(t, log, "")

	if exitErr := EnsureProjectDeps(context.Background(), root); exitErr != nil {
		t.Fatalf("unexpected error: %v", exitErr)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Error("npm must not run when node_modules already exists")
	}
}

func TestEnsureProjectDeps_SkipsWhenAncestorHasNodeModules(t *testing.T) {
	// Module resolution walks upward, so an ancestor node_modules already
	// satisfies imports — no install needed (e.g. cwd is extensions/<id>).
	parent := t.TempDir()
	if err := os.Mkdir(filepath.Join(parent, "node_modules"), 0o755); err != nil {
		t.Fatal(err)
	}
	child := filepath.Join(parent, "extensions", "demo")
	if err := os.MkdirAll(child, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(child, "package.json"), []byte(`{"name":"demo"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	log := filepath.Join(t.TempDir(), "npm.log")
	fakeNpm(t, log, "")

	if exitErr := EnsureProjectDeps(context.Background(), child); exitErr != nil {
		t.Fatalf("unexpected error: %v", exitErr)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Error("npm must not run when an ancestor dir has node_modules")
	}
}

func TestEnsureProjectDeps_SkipsWhenNoPackageJSON(t *testing.T) {
	root := t.TempDir() // no package.json
	log := filepath.Join(t.TempDir(), "npm.log")
	fakeNpm(t, log, "")

	if exitErr := EnsureProjectDeps(context.Background(), root); exitErr != nil {
		t.Fatalf("unexpected error: %v", exitErr)
	}
	if _, err := os.Stat(log); !os.IsNotExist(err) {
		t.Error("npm must not run without a package.json")
	}
}

func TestEnsureProjectDeps_SkipsWhenNpmNotOnPath(t *testing.T) {
	root := newProject(t)
	t.Setenv("PATH", t.TempDir()) // empty dir: no npm

	// Missing npm is not fatal here: the build proceeds and, on failure, the
	// unresolved-import validation hint guides the user.
	if exitErr := EnsureProjectDeps(context.Background(), root); exitErr != nil {
		t.Fatalf("missing npm must be a silent skip, got %v", exitErr)
	}
}

func TestEnsureProjectDeps_InstallFailure_IsValidationWithHint(t *testing.T) {
	root := newProject(t)
	log := filepath.Join(t.TempDir(), "npm.log")
	fakeNpm(t, log, "exit 1")

	exitErr := EnsureProjectDeps(context.Background(), root)
	if exitErr == nil {
		t.Fatal("expected an ExitError when npm install fails")
	}
	if exitErr.Code != output.ExitValidation || exitErr.Detail.Type != output.TypeValidation {
		t.Errorf("got code=%d type=%q, want validation", exitErr.Code, exitErr.Detail.Type)
	}
	if !strings.Contains(exitErr.Detail.Hint, "npm install") {
		t.Errorf("hint %q should point at running npm install manually", exitErr.Detail.Hint)
	}
}
