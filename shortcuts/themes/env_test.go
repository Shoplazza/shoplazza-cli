package themes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/themeenv"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const envSample = `
[environments.default]
store = "my-dev.myshoplaza.com"
[environments.staging]
store = "staging.myshoplaza.com"
theme = "123456"
profile = "staging"
`

// envFlags builds a FlagSet exposing --path pointed at dir (where the test wrote
// its shoplazza.theme.toml).
func envFlags(dir string) common.FlagSet {
	cmd := &cobra.Command{Use: "env"}
	cmd.Flags().String("path", dir, "")
	return common.NewCobraFlagSet(cmd)
}

func writeEnvFile(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, themeenv.FileName), []byte(envSample), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestEnvList_ReturnsSortedEnvironments(t *testing.T) {
	dir := writeEnvFile(t)
	res, err := envListShortcut.Execute(context.Background(), common.ExecInput{Flags: envFlags(dir)})
	if err != nil {
		t.Fatalf("env list: %v", err)
	}
	envs, ok := res.Body["environments"].([]map[string]any)
	if !ok {
		t.Fatalf("environments has wrong type: %T", res.Body["environments"])
	}
	if len(envs) != 2 {
		t.Fatalf("got %d environments, want 2", len(envs))
	}
	if envs[0]["name"] != "default" || envs[1]["name"] != "staging" {
		t.Errorf("environments not sorted by name: %v", envs)
	}
}

func TestEnvShow_NamedAndDefault(t *testing.T) {
	dir := writeEnvFile(t)

	res, err := envShowShortcut.Execute(context.Background(), common.ExecInput{Args: []string{"staging"}, Flags: envFlags(dir)})
	if err != nil {
		t.Fatalf("env show staging: %v", err)
	}
	if res.Body["store"] != "staging.myshoplaza.com" || res.Body["theme"] != "123456" {
		t.Errorf("staging body = %v", res.Body)
	}

	// No name → the default environment.
	res, err = envShowShortcut.Execute(context.Background(), common.ExecInput{Flags: envFlags(dir)})
	if err != nil {
		t.Fatalf("env show (default): %v", err)
	}
	if res.Body["name"] != "default" || res.Body["store"] != "my-dev.myshoplaza.com" {
		t.Errorf("default body = %v", res.Body)
	}
}

func TestEnvShow_MissingIsStructuredValidationError(t *testing.T) {
	dir := writeEnvFile(t)
	_, err := envShowShortcut.Execute(context.Background(), common.ExecInput{Args: []string{"nope"}, Flags: envFlags(dir)})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation ExitError, got %v", err)
	}
	if ee.Detail == nil || ee.Detail.Hint == "" {
		t.Errorf("missing-environment error should carry a hint listing the defined names: %+v", ee.Detail)
	}
}

func TestCheckEnvironment(t *testing.T) {
	cfg := &core.CliConfig{ConfigVersion: 2, Profiles: []core.ProfileConfig{
		{Name: "staging", StoreDomain: "staging.myshoplaza.com"},
	}}
	for _, tc := range []struct {
		name    string
		env     themeenv.Environment
		wantOK  bool
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
				t.Errorf("ok=%v, want %v (issues: %v)", len(issues) == 0, tc.wantOK, issues)
			}
		})
	}
}

func TestEnvCheck_FailsWhenAnyEnvironmentIsInvalid(t *testing.T) {
	dir := t.TempDir()
	body := "[environments.ok]\ntheme = \"123\"\n[environments.bad]\nprofile = \"ghost\"\n"
	if err := os.WriteFile(filepath.Join(dir, themeenv.FileName), []byte(body), 0o644); err != nil {
		t.Fatal(err)
	}
	_, err := envCheckShortcut.Execute(context.Background(), common.ExecInput{Flags: envFlags(dir)})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation ExitError naming the bad env, got %v", err)
	}
}

func TestEnvList_NoFileIsStructuredError(t *testing.T) {
	// A bare dir with no shoplazza.theme.toml anywhere up the tree.
	_, err := envListShortcut.Execute(context.Background(), common.ExecInput{Flags: envFlags(t.TempDir())})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation ExitError for a missing file, got %v", err)
	}
}
