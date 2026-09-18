package themecmd

import (
	"bytes"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

// runEnv drives an env subcommand's RunE with the given flags + args, capturing
// output. Flags bind to the command's closure vars, so Set updates them.
func runEnv(t *testing.T, cmd *cobra.Command, args []string, flags map[string]string) error {
	t.Helper()
	for k, v := range flags {
		if err := cmd.Flags().Set(k, v); err != nil {
			t.Fatalf("set --%s: %v", k, err)
		}
	}
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	return cmd.RunE(cmd, args)
}

// chdir switches the working directory to dir for the test and restores it after.
// env commands operate on the cwd's shoplazza.theme.toml (there is no --path),
// so tests run inside a temp project dir.
func chdir(t *testing.T, dir string) {
	t.Helper()
	old, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}
	if err := os.Chdir(dir); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chdir(old) })
}

func TestCheckEnvironment(t *testing.T) {
	cfg := &core.CliConfig{ConfigVersion: 2, Profiles: []core.ProfileConfig{
		{Name: "staging", StoreDomain: "staging.myshoplaza.com"},
	}}
	for _, tc := range []struct {
		name   string
		env    env.Environment
		wantOK bool
	}{
		{"configured profile", env.Environment{Profile: "staging"}, true},
		{"store with authenticated profile", env.Environment{Store: "staging.myshoplaza.com"}, true},
		{"theme-only rides current store", env.Environment{Theme: "123"}, true},
		{"unconfigured profile", env.Environment{Profile: "ghost"}, false},
		{"store with no profile", env.Environment{Store: "nobody.myshoplaza.com"}, false},
		{"empty block", env.Environment{}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			issues := checkEnvironment(cfg, tc.env)
			if (len(issues) == 0) != tc.wantOK {
				t.Errorf("ok=%v want %v (issues: %v)", len(issues) == 0, tc.wantOK, issues)
			}
		})
	}
}

func TestEnvAdd_SetOnlyChangesGivenFields(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	f := &cmdutil.Factory{}

	if err := runEnv(t, newCmdEnvAdd(f), []string{"staging"},
		map[string]string{"store": "staging.myshoplaza.com", "theme": "123"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	file, err := env.Load(filepath.Join(dir, env.FileName))
	if err != nil {
		t.Fatalf("load after add: %v", err)
	}
	if e, _ := file.Environment("staging"); e.Store != "staging.myshoplaza.com" || e.Theme != "123" {
		t.Fatalf("added env = %+v", e)
	}

	// add again → duplicate error.
	if err := runEnv(t, newCmdEnvAdd(f), []string{"staging"}, map[string]string{"store": "x"}); err == nil {
		t.Error("adding an existing environment must error")
	}

	// set only --theme: store preserved, theme changes.
	if err := runEnv(t, newCmdEnvSet(f), []string{"staging"}, map[string]string{"theme": "999"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	file, _ = env.Load(filepath.Join(dir, env.FileName))
	if e, _ := file.Environment("staging"); e.Theme != "999" || e.Store != "staging.myshoplaza.com" {
		t.Errorf("after set: theme/store = %q/%q, want 999 with store preserved", e.Theme, e.Store)
	}
}

// TestEnvAdd_RequiresNameNonInteractively: an agent that omits the name gets a
// structured error (not cobra's arg-count message) and writes nothing.
func TestEnvAdd_RequiresNameNonInteractively(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	f := &cmdutil.Factory{} // non-interactive

	err := runEnv(t, newCmdEnvAdd(f), nil, map[string]string{"store": "s.myshoplaza.com"})
	if err == nil {
		t.Fatal("expected an error when the name is omitted non-interactively")
	}
	if _, lerr := env.Load(filepath.Join(dir, env.FileName)); lerr == nil {
		t.Fatal("no shoplazza.theme.toml should have been written")
	}
}

// TestEnvAdd_RequiresStoreNonInteractively: an agent that omits --store must get
// a structured "required flag" error, not a silently-empty environment (and no
// file is written).
func TestEnvAdd_RequiresStoreNonInteractively(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	f := &cmdutil.Factory{} // non-interactive

	err := runEnv(t, newCmdEnvAdd(f), []string{"staging"}, nil)
	if err == nil {
		t.Fatal("expected an error when --store is omitted non-interactively")
	}
	if _, lerr := env.Load(filepath.Join(dir, env.FileName)); lerr == nil {
		t.Fatal("no shoplazza.theme.toml should have been written")
	}
}

// TestResolveEnvName covers name resolution for set/remove: an explicit arg wins;
// non-interactively an omitted name errors (agents must name it); an empty file
// errors regardless.
func TestResolveEnvName(t *testing.T) {
	f := &cmdutil.Factory{} // non-interactive
	file := env.File{Environments: map[string]env.Environment{
		"staging": {Store: "s.myshoplaza.com"},
		"prod":    {Store: "p.myshoplaza.com"},
	}}

	// Explicit arg is returned verbatim.
	if name, err := resolveEnvName(f, file, []string{"prod"}); err != nil || name != "prod" {
		t.Fatalf("explicit arg: name=%q err=%v", name, err)
	}
	// Omitted name, non-interactive → error naming the choices.
	if _, err := resolveEnvName(f, file, nil); err == nil {
		t.Fatal("omitted name non-interactively must error")
	}
	// Empty file, no name → error.
	if _, err := resolveEnvName(f, env.File{}, nil); err == nil {
		t.Fatal("empty file with no name must error")
	}
}

// TestProfilePickerChoices: env add's --profile picker lists a skip option plus
// one option per configured profile; no profiles → nil (text-input fallthrough).
func TestProfilePickerChoices(t *testing.T) {
	// No profiles configured → nil (ResolveFlags falls through to text input).
	if opts, err := profilePickerChoices(t.Context(), nil, &cmdutil.Factory{}); err != nil || opts != nil {
		t.Fatalf("empty config: opts=%v err=%v, want nil/nil", opts, err)
	}

	f := &cmdutil.Factory{Config: core.CliConfig{Profiles: []core.ProfileConfig{
		{Name: "prod", StoreDomain: "prod.myshoplaza.com"},
		{Name: "staging", StoreDomain: "staging.myshoplaza.com"},
	}}}
	opts, err := profilePickerChoices(t.Context(), nil, f)
	if err != nil {
		t.Fatalf("picker: %v", err)
	}
	if len(opts) != 3 {
		t.Fatalf("want 3 options (skip + 2 profiles), got %d: %+v", len(opts), opts)
	}
	if opts[0].Value != "" {
		t.Errorf("first option must be the skip sentinel (empty value), got %q", opts[0].Value)
	}
	if opts[1].Value != "prod" || opts[2].Value != "staging" {
		t.Errorf("profile values = %q, %q; want prod, staging", opts[1].Value, opts[2].Value)
	}
}

// TestEnvSetRemove_RequireNameNonInteractively: set/remove with no name and no
// TTY fail-fast instead of hanging on a picker.
func TestEnvSetRemove_RequireNameNonInteractively(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	f := &cmdutil.Factory{}
	if err := runEnv(t, newCmdEnvAdd(f), []string{"staging"},
		map[string]string{"store": "staging.myshoplaza.com"}); err != nil {
		t.Fatalf("seed add: %v", err)
	}

	if err := runEnv(t, newCmdEnvSet(f), nil, map[string]string{"theme": "9"}); err == nil {
		t.Error("env set with no name non-interactively must error")
	}
	if err := runEnv(t, newCmdEnvRemove(f), nil, nil); err == nil {
		t.Error("env remove with no name non-interactively must error")
	}
	// The seeded environment must still be intact (nothing was removed).
	file, _ := env.Load(filepath.Join(dir, env.FileName))
	if _, ok := file.Environment("staging"); !ok {
		t.Fatal("staging environment was unexpectedly removed")
	}
}

func TestEnvSet_MissingEnvironmentErrors(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeEnvFixture(t, dir)
	err := runEnv(t, newCmdEnvSet(&cmdutil.Factory{}), []string{"ghost"}, map[string]string{"theme": "1"})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("set on a missing env must be a validation error, got %v", err)
	}
}

func TestEnvRemove(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	writeEnvFixture(t, dir)
	if err := runEnv(t, newCmdEnvRemove(&cmdutil.Factory{}), []string{"staging"}, nil); err != nil {
		t.Fatalf("remove: %v", err)
	}
	file, _ := env.Load(filepath.Join(dir, env.FileName))
	if _, ok := file.Environment("staging"); ok {
		t.Error("staging should be gone")
	}
	if _, ok := file.Environment("default"); !ok {
		t.Error("default must survive")
	}
	if err := runEnv(t, newCmdEnvRemove(&cmdutil.Factory{}), []string{"nope"}, nil); err == nil {
		t.Error("removing a missing env must error")
	}
}

func TestEnvCheck_FailsWhenAnyInvalid(t *testing.T) {
	dir := t.TempDir()
	chdir(t, dir)
	body := "[environments.ok]\ntheme = \"123\"\n[environments.bad]\nprofile = \"ghost\"\n"
	if err := os.WriteFile(filepath.Join(dir, env.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runEnv(t, newCmdEnvCheck(&cmdutil.Factory{}), nil, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("check must fail (validation) when an env is invalid, got %v", err)
	}
}

func TestEnvList_NoFileIsStructuredError(t *testing.T) {
	chdir(t, t.TempDir())
	err := runEnv(t, newCmdEnvList(&cmdutil.Factory{}), nil, nil)
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("list with no file must be a validation error, got %v", err)
	}
}

func writeEnvFixture(t *testing.T, dir string) {
	t.Helper()
	body := "[environments.default]\nstore = \"my-dev.myshoplaza.com\"\n[environments.staging]\nstore = \"staging.myshoplaza.com\"\ntheme = \"123\"\nprofile = \"staging\"\n"
	if err := os.WriteFile(filepath.Join(dir, env.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
