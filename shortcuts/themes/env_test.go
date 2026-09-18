package themes

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

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

func TestEnvList_NoFileIsStructuredError(t *testing.T) {
	// A bare dir with no shoplazza.theme.toml anywhere up the tree.
	_, err := envListShortcut.Execute(context.Background(), common.ExecInput{Flags: envFlags(t.TempDir())})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("want a validation ExitError for a missing file, got %v", err)
	}
}
