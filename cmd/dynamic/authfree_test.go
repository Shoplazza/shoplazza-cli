package dynamic

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/core"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/internal/registry"
	"shoplazza-cli-v2/shortcuts/common"

	"github.com/spf13/cobra"
)

// notLoggedInFactory builds a Factory with a fresh (empty) keychain and no
// SHOPLAZZA_ACCESS_TOKEN, so cmdutil.RequireAuth reports "not logged in".
// Mirrors tempFactory in internal/cmdutil/require_auth_test.go.
func notLoggedInFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
	t.Setenv("HOME", dir)
	out := &strings.Builder{}
	return &cmdutil.Factory{
		IOStreams:  cmdutil.IOStreams{In: strings.NewReader(""), Out: out, ErrOut: &strings.Builder{}},
		ConfigPath: filepath.Join(dir, "config.json"),
		Config:     core.CliConfig{},
		Client:     client.New("http://127.0.0.1:1"),
		AuthClient: client.New("http://127.0.0.1:1"),
	}
}

// themesSpec is a minimal spec with one valid spec-generated leaf so
// buildModuleCommand produces a real module node (the auth gate host).
func themesSpec() *registry.Spec {
	return &registry.Spec{Modules: []registry.Module{{
		Name: "themes",
		Commands: []registry.Command{{
			ID: "theme-list", Path: []string{"list"},
			HTTP: registry.HTTP{Method: "GET", Path: "/openapi/2026-01/themes"},
		}},
	}}}
}

// mountUnderThemes registers the spec module on a fresh root and mounts s
// under the resulting "themes" module command — the same wiring production
// uses (RegisterCommands then RegisterShortcuts finds the existing group).
func mountUnderThemes(t *testing.T, f *cmdutil.Factory, s common.Shortcut) *cobra.Command {
	t.Helper()
	root := rootWithSpec(t, themesSpec(), f)
	var themesCmd *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "themes" {
			themesCmd = c
		}
	}
	if themesCmd == nil {
		t.Fatal("themes module command not registered")
	}
	common.Mount(s, themesCmd, f)
	return root
}

// TestAuthGate_AuthFreeShortcutRunsWithoutLogin: a Shortcut declaring
// AuthFree (themes init / package) must execute with no credentials at all —
// previously even --dry-run exited 3.
func TestAuthGate_AuthFreeShortcutRunsWithoutLogin(t *testing.T) {
	f := notLoggedInFactory(t)
	ran := false
	s := common.Shortcut{
		Service: "themes", Command: "local-probe", Use: "local-probe", Short: "p",
		AuthFree: true, Local: true,
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			ran = true
			return common.ExecResult{Body: map[string]any{"status": "ok"}}, nil
		},
	}
	root := mountUnderThemes(t, f, s)
	root.SetArgs([]string{"themes", "local-probe"})
	if err := root.Execute(); err != nil {
		t.Fatalf("AuthFree shortcut must run without login; got: %v", err)
	}
	if !ran {
		t.Fatal("Execute handler never ran")
	}
}

// TestAuthGate_NormalShortcutStillGated: a shortcut WITHOUT AuthFree under the
// same module must keep hitting the auth gate (type=auth, "not logged in").
func TestAuthGate_NormalShortcutStillGated(t *testing.T) {
	f := notLoggedInFactory(t)
	s := common.Shortcut{
		Service: "themes", Command: "gated-probe", Use: "gated-probe", Short: "p",
		Execute: func(_ context.Context, _ common.ExecInput) (common.ExecResult, error) {
			t.Error("gated shortcut must not execute without login")
			return common.ExecResult{}, nil
		},
	}
	root := mountUnderThemes(t, f, s)
	root.SetArgs([]string{"themes", "gated-probe"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected auth-gate error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("expected type=auth ExitError, got %T: %v", err, err)
	}
}

// TestAuthGate_SpecLeafStillGated: spec-generated leaves carry no AuthFree
// annotation and must remain gated.
func TestAuthGate_SpecLeafStillGated(t *testing.T) {
	f := notLoggedInFactory(t)
	root := rootWithSpec(t, themesSpec(), f)
	root.SetArgs([]string{"themes", "list"})
	err := root.Execute()
	if err == nil {
		t.Fatal("expected auth-gate error for the spec leaf")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeAuth {
		t.Fatalf("expected type=auth ExitError, got %T: %v", err, err)
	}
}
