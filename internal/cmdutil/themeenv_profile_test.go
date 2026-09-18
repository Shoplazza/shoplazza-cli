package cmdutil

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/themeenv"
)

const themeEnvFixture = `
[environments.staging]
store   = "staging.myshoplaza.com"
profile = "staging-profile"

[environments.by-store]
store = "prod.myshoplaza.com"

[environments.orphan-store]
store = "nobody.myshoplaza.com"

[environments.ghost-profile]
profile = "does-not-exist"
`

// newThemeCmd mirrors a theme command's merged flag set (--profile from the
// global set, -e/--environment + --path from the shortcut) by the time
// ResolveProfile runs. path seeds the upward search for the fixture file.
func newThemeCmd(dir string) *cobra.Command {
	cmd := &cobra.Command{Use: "push"}
	cmd.Flags().String("profile", "", "")
	cmd.Flags().StringP(EnvironmentFlag, "e", "", "")
	cmd.Flags().String("path", dir, "")
	return cmd
}

func themeEnvFactory() *Factory {
	return &Factory{Config: core.CliConfig{ConfigVersion: 2, CurrentProfile: "current",
		Profiles: []core.ProfileConfig{
			{Name: "current", StoreDomain: "dev.myshoplaza.com"},
			{Name: "staging-profile", StoreDomain: "staging.myshoplaza.com"},
			{Name: "prod-owner", StoreDomain: "prod.myshoplaza.com"},
		}}}
}

func writeThemeEnv(t *testing.T) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, themeenv.FileName), []byte(themeEnvFixture), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

func TestResolveProfile_Environment(t *testing.T) {
	dir := writeThemeEnv(t)
	f := themeEnvFactory()
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	t.Setenv(EnvironmentVar, "")

	// env profile= wins over the current profile.
	cmd := newThemeCmd(dir)
	_ = cmd.Flags().Set(EnvironmentFlag, "staging")
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "staging-profile" {
		t.Fatalf("env profile=: got %v, %v", p, err)
	}

	// env store= resolves via the matching authenticated profile.
	cmd = newThemeCmd(dir)
	_ = cmd.Flags().Set(EnvironmentFlag, "by-store")
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "prod-owner" {
		t.Fatalf("env store=: got %v, %v", p, err)
	}

	// SHOPLAZZA_CLI_ENVIRONMENT is the CI equivalent of -e.
	cmd = newThemeCmd(dir)
	t.Setenv(EnvironmentVar, "staging")
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "staging-profile" {
		t.Fatalf("SHOPLAZZA_CLI_ENVIRONMENT: got %v, %v", p, err)
	}
	t.Setenv(EnvironmentVar, "")
}

func TestResolveProfile_ExplicitProfileBeatsEnvironment(t *testing.T) {
	dir := writeThemeEnv(t)
	f := themeEnvFactory()
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	cmd := newThemeCmd(dir)
	_ = cmd.Flags().Set(EnvironmentFlag, "staging")
	_ = cmd.Flags().Set("profile", "current")
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "current" {
		t.Fatalf("explicit --profile must beat -e: got %v, %v", p, err)
	}
}

func TestResolveProfile_EnvironmentErrors(t *testing.T) {
	dir := writeThemeEnv(t)
	f := themeEnvFactory()
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	t.Setenv(EnvironmentVar, "")

	for _, tc := range []struct{ name, env string }{
		{"unknown environment", "nope"},
		{"env names a store with no authenticated profile", "orphan-store"},
		{"env names a profile that is not configured", "ghost-profile"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cmd := newThemeCmd(dir)
			_ = cmd.Flags().Set(EnvironmentFlag, tc.env)
			if _, err := ResolveProfile(f, cmd); err == nil {
				t.Fatal("expected a structured error, got nil")
			}
		})
	}

	// -e given but no shoplazza.theme.toml anywhere → structured error.
	bare := newThemeCmd(t.TempDir())
	_ = bare.Flags().Set(EnvironmentFlag, "staging")
	if _, err := ResolveProfile(f, bare); err == nil {
		t.Fatal("-e with no theme file must error")
	}
}

// TestResolveProfile_NonThemeCommandIgnoresEnvironmentVar pins the gate: a
// command without an --environment flag never consults the theme env file, even
// with SHOPLAZZA_CLI_ENVIRONMENT set — so non-theme commands are byte-for-byte
// unchanged.
func TestResolveProfile_NonThemeCommandIgnoresEnvironmentVar(t *testing.T) {
	_ = writeThemeEnv(t)
	f := themeEnvFactory()
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	t.Setenv(EnvironmentVar, "staging")

	cmd := newCmdWithProfileFlag() // no --environment flag
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "current" {
		t.Fatalf("non-theme command must ignore the env var and use the current profile: got %v, %v", p, err)
	}
}
