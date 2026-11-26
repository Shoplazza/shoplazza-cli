package themecmd

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
)

func TestResolveStore_EmptyEnvToken(t *testing.T) {
	dir := testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_ACCESS_TOKEN", "")
	f := &cmdutil.Factory{
		ConfigPath: filepath.Join(dir, "config.json"),
		Config: core.CliConfig{ConfigVersion: 2, CurrentProfile: "us",
			Profiles: []core.ProfileConfig{{Name: "us", Account: "a@co.com", StoreDomain: "us.myshoplazza.com"}}},
		AuthClient: client.New("http://127.0.0.1:1"),
	}
	_, err := resolveStore(context.Background(), f, &cobra.Command{})
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation {
		t.Fatalf("SHOPLAZZA_ACCESS_TOKEN set but empty must be a validation error, not a fall back to the profile; got %v", err)
	}
}

// TestDefaultEnvironmentAt covers auto-apply resolution for no-`-e` commands:
// a committed [environments.default] is returned; a missing file or a file with
// no default resolves to "not found" (ok=false, no error); a malformed file
// surfaces an error rather than silently falling back.
func TestDefaultEnvironmentAt(t *testing.T) {
	t.Run("no file → ok=false, no error", func(t *testing.T) {
		e, ok, err := defaultEnvironmentAt(t.TempDir())
		if err != nil || ok {
			t.Fatalf("got e=%+v ok=%v err=%v, want zero/false/nil", e, ok, err)
		}
	})

	t.Run("file with default → returned", func(t *testing.T) {
		dir := t.TempDir()
		seed := env.File{Environments: map[string]env.Environment{
			"default": {Store: "myshop.myshoplaza.com", Theme: "123"},
			"staging": {Store: "staging.myshoplaza.com"},
		}}
		if err := env.Save(filepath.Join(dir, env.FileName), seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
		e, ok, err := defaultEnvironmentAt(dir)
		if err != nil || !ok {
			t.Fatalf("ok=%v err=%v, want true/nil", ok, err)
		}
		if e.Store != "myshop.myshoplaza.com" || e.Theme != "123" {
			t.Fatalf("default env = %+v", e)
		}
	})

	t.Run("file without default → ok=false", func(t *testing.T) {
		dir := t.TempDir()
		seed := env.File{Environments: map[string]env.Environment{
			"staging": {Store: "staging.myshoplaza.com"},
		}}
		if err := env.Save(filepath.Join(dir, env.FileName), seed); err != nil {
			t.Fatalf("seed: %v", err)
		}
		_, ok, err := defaultEnvironmentAt(dir)
		if err != nil || ok {
			t.Fatalf("ok=%v err=%v, want false/nil", ok, err)
		}
	})

	t.Run("malformed file → error", func(t *testing.T) {
		dir := t.TempDir()
		if err := os.WriteFile(filepath.Join(dir, env.FileName), []byte("this is = = not valid toml ]["), 0o644); err != nil {
			t.Fatalf("write: %v", err)
		}
		if _, _, err := defaultEnvironmentAt(dir); err == nil {
			t.Fatal("malformed toml must surface an error, not silently fall back")
		}
	})
}
