package appcmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/keychain"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

func TestExtractFunctionsHandlesNesting(t *testing.T) {
	// Realistic envelope: top-level {code, data:{data:[...]}} (double data, as v1 read).
	body := map[string]any{
		"code": "SUCCESS",
		"data": map[string]any{
			"data": []any{
				map[string]any{"function_id": "fn_1", "name": "a", "source_code": "SECRET"},
				map[string]any{"function_id": "fn_2", "name": "b"},
			},
		},
	}
	fns := extractFunctions(body)
	if len(fns) != 2 {
		t.Fatalf("expected 2 functions, got %d", len(fns))
	}
	if _, leaked := fns[0]["source_code"]; leaked {
		t.Error("source_code must be stripped from list output")
	}
	if fns[0]["function_id"] != "fn_1" {
		t.Errorf("unexpected first function: %v", fns[0])
	}
}

func TestFunctionReleaseRequiresExtension(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := newCmdFunctionRelease(f)
	cmd.SetArgs([]string{})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error when --extension missing, got %v", err)
	}
}

func TestDigToArrayShapes(t *testing.T) {
	want := func(t *testing.T, arr []any) {
		t.Helper()
		if len(arr) != 1 {
			t.Fatalf("expected 1 element, got %d (%v)", len(arr), arr)
		}
	}
	// bare array
	want(t, digToArray([]any{map[string]any{"x": 1}}))
	// single {data:[...]}
	want(t, digToArray(map[string]any{"data": []any{map[string]any{"x": 1}}}))
	// real 2024-07 shape {data:{functions:[...]}} (dumped from dev)
	want(t, digToArray(map[string]any{"data": map[string]any{"functions": []any{map[string]any{"x": 1}}}}))
	// double {data:{data:[...]}} (legacy fallback)
	want(t, digToArray(map[string]any{"data": map[string]any{"data": []any{map[string]any{"x": 1}}}}))
	// top-level {functions:[...]} — what unmarshalUnwrapped leaves after
	// stripping a code:"Success" envelope
	want(t, digToArray(map[string]any{"functions": []any{map[string]any{"x": 1}}, "total": 1}))
	// unrecognized shape → nil
	if got := digToArray(map[string]any{"nope": 1}); got != nil {
		t.Fatalf("expected nil for unrecognized shape, got %v", got)
	}
}

// TestFunction_ExtensionTraversalRejected: --extension is joined under
// extensions/, so path-y values must fail validation before any FS access.
func TestFunction_ExtensionTraversalRejected(t *testing.T) {
	for _, newCmd := range []func(*cmdutil.Factory) *cobra.Command{newCmdFunctionCompile, newCmdFunctionRelease} {
		cmd := newCmd(&cmdutil.Factory{})
		cmd.SetArgs([]string{"--name", "../../etc", "--path", t.TempDir()})
		err := cmd.Execute()
		var ee *output.ExitError
		if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
			t.Errorf("%s: expected validation error for traversal --name, got %v", cmd.Use, err)
		}
	}
}

func TestFunctionCompileMissingEntry(t *testing.T) {
	root := t.TempDir()
	// extensions/foo exists but has no src/index.js
	if err := os.MkdirAll(filepath.Join(root, "extensions", "foo"), 0o755); err != nil {
		t.Fatal(err)
	}
	f := &cmdutil.Factory{}
	cmd := newCmdFunctionCompile(f)
	cmd.SetArgs([]string{"--name", "foo", "--path", root})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error for missing entry, got %v", err)
	}
}

func TestFunctionListNoCurrentApp(t *testing.T) {
	// Isolate keychain to a temp dir and seed UAT + partner token so requireLogin
	// passes — the test checks the no-current-app branch, not the auth gate.
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountUATKey("alice@co.com"), "uat_1"); err != nil {
		t.Fatalf("keychain Set uat: %v", err)
	}
	if err := keychain.Set(keychain.ShoplazzaCliService, internalauth.AccountPartnerKey("alice@co.com"), "ptok_1"); err != nil {
		t.Fatalf("keychain Set partner: %v", err)
	}

	// empty project: no shoplazza.app.toml → p.ActiveConfig() read fails → validation
	root := t.TempDir()
	f := &cmdutil.Factory{
		Config:     core.CliConfig{Accounts: []core.AccountConfig{{Name: "alice@co.com"}}},
		ConfigPath: filepath.Join(dir, "config.json"),
		AuthClient: client.New("https://partners.example.com"),
	}
	cmd := newCmdFunctionList(f)
	cmd.SetArgs([]string{"--path", root})
	err := cmd.Execute()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Detail == nil || ee.Detail.Type != output.TypeValidation {
		t.Fatalf("expected validation error, got %v", err)
	}
}

func TestNextPatchVersion(t *testing.T) {
	cases := map[string]string{
		"1.0.0": "1.0.1",
		"1.0.9": "1.0.10",
		"2.3.4": "2.3.5",
		"7":     "8",
		"":      "1.0.0", // empty → seed
		"1.0.x": "1.0.0", // non-numeric tail → seed
	}
	for in, want := range cases {
		if got := nextPatchVersion(in); got != want {
			t.Errorf("nextPatchVersion(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestFunctionGroupRegistered(t *testing.T) {
	f := &cmdutil.Factory{}
	root := NewCmdApp(f)
	var buf bytes.Buffer
	root.SetOut(&buf)
	root.SetArgs([]string{"function", "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("app function --help: %v", err)
	}
	out := buf.String()
	for _, sub := range []string{"compile", "release", "list"} {
		if !strings.Contains(out, sub) {
			t.Errorf("expected %q in `app function --help`, got:\n%s", sub, out)
		}
	}
}
