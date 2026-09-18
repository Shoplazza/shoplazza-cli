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
	"github.com/Shoplazza/shoplazza-cli/v2/internal/themeenv"
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

func TestCheckEnvironment(t *testing.T) {
	cfg := &core.CliConfig{ConfigVersion: 2, Profiles: []core.ProfileConfig{
		{Name: "staging", StoreDomain: "staging.myshoplaza.com"},
	}}
	for _, tc := range []struct {
		name   string
		env    themeenv.Environment
		wantOK bool
	}{
		{"configured profile", themeenv.Environment{Profile: "staging"}, true},
		{"store with authenticated profile", themeenv.Environment{Store: "staging.myshoplaza.com"}, true},
		{"theme-only rides current store", themeenv.Environment{Theme: "123"}, true},
		{"unconfigured profile", themeenv.Environment{Profile: "ghost"}, false},
		{"store with no profile", themeenv.Environment{Store: "nobody.myshoplaza.com"}, false},
		{"empty block", themeenv.Environment{}, false},
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
	f := &cmdutil.Factory{}

	if err := runEnv(t, newCmdEnvAdd(f), []string{"staging"},
		map[string]string{"path": dir, "store": "staging.myshoplaza.com", "theme": "123"}); err != nil {
		t.Fatalf("add: %v", err)
	}
	file, err := themeenv.Load(filepath.Join(dir, themeenv.FileName))
	if err != nil {
		t.Fatalf("load after add: %v", err)
	}
	if e, _ := file.Environment("staging"); e.Store != "staging.myshoplaza.com" || e.Theme != "123" {
		t.Fatalf("added env = %+v", e)
	}

	// add again → duplicate error.
	if err := runEnv(t, newCmdEnvAdd(f), []string{"staging"}, map[string]string{"path": dir, "store": "x"}); err == nil {
		t.Error("adding an existing environment must error")
	}

	// set only --theme: store preserved, theme changes.
	if err := runEnv(t, newCmdEnvSet(f), []string{"staging"}, map[string]string{"path": dir, "theme": "999"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	file, _ = themeenv.Load(filepath.Join(dir, themeenv.FileName))
	if e, _ := file.Environment("staging"); e.Theme != "999" || e.Store != "staging.myshoplaza.com" {
		t.Errorf("after set: theme/store = %q/%q, want 999 with store preserved", e.Theme, e.Store)
	}
}

func TestEnvSet_MissingEnvironmentErrors(t *testing.T) {
	dir := t.TempDir()
	writeEnvFixture(t, dir)
	err := runEnv(t, newCmdEnvSet(&cmdutil.Factory{}), []string{"ghost"}, map[string]string{"path": dir, "theme": "1"})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("set on a missing env must be a validation error, got %v", err)
	}
}

func TestEnvRemove(t *testing.T) {
	dir := t.TempDir()
	writeEnvFixture(t, dir)
	if err := runEnv(t, newCmdEnvRemove(&cmdutil.Factory{}), []string{"staging"}, map[string]string{"path": dir}); err != nil {
		t.Fatalf("remove: %v", err)
	}
	file, _ := themeenv.Load(filepath.Join(dir, themeenv.FileName))
	if _, ok := file.Environment("staging"); ok {
		t.Error("staging should be gone")
	}
	if _, ok := file.Environment("default"); !ok {
		t.Error("default must survive")
	}
	if err := runEnv(t, newCmdEnvRemove(&cmdutil.Factory{}), []string{"nope"}, map[string]string{"path": dir}); err == nil {
		t.Error("removing a missing env must error")
	}
}

func TestEnvCheck_FailsWhenAnyInvalid(t *testing.T) {
	dir := t.TempDir()
	body := "[environments.ok]\ntheme = \"123\"\n[environments.bad]\nprofile = \"ghost\"\n"
	if err := os.WriteFile(filepath.Join(dir, themeenv.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	err := runEnv(t, newCmdEnvCheck(&cmdutil.Factory{}), nil, map[string]string{"path": dir})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("check must fail (validation) when an env is invalid, got %v", err)
	}
}

func TestEnvList_NoFileIsStructuredError(t *testing.T) {
	err := runEnv(t, newCmdEnvList(&cmdutil.Factory{}), nil, map[string]string{"path": t.TempDir()})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("list with no file must be a validation error, got %v", err)
	}
}

func writeEnvFixture(t *testing.T, dir string) {
	t.Helper()
	body := "[environments.default]\nstore = \"my-dev.myshoplaza.com\"\n[environments.staging]\nstore = \"staging.myshoplaza.com\"\ntheme = \"123\"\nprofile = \"staging\"\n"
	if err := os.WriteFile(filepath.Join(dir, themeenv.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
}
