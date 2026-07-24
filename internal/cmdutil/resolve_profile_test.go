package cmdutil

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"

	"github.com/spf13/cobra"
)

// newCmdWithProfileFlag returns a bare cobra.Command carrying a local
// --profile flag, mirroring what the merged persistent flag looks like by
// the time RequireAuth/ResolveProfile run in production.
func newCmdWithProfileFlag() *cobra.Command {
	cmd := &cobra.Command{Use: "test"}
	cmd.Flags().String("profile", "", "")
	return cmd
}

func TestResolveProfile_Priority(t *testing.T) {
	cfg := core.CliConfig{ConfigVersion: 2, CurrentProfile: "cfg",
		Profiles: []core.ProfileConfig{{Name: "cfg"}, {Name: "env"}, {Name: "flag"}}}
	f := &Factory{Config: cfg}
	cmd := newCmdWithProfileFlag()

	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	p, err := ResolveProfile(f, cmd)
	if err != nil || p.Name != "cfg" {
		t.Fatalf("config level: %v %v", p, err)
	}
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "env")
	if p, _ = ResolveProfile(f, cmd); p.Name != "env" {
		t.Fatal("env beats config")
	}
	_ = cmd.Flags().Set("profile", "flag")
	if p, _ = ResolveProfile(f, cmd); p.Name != "flag" {
		t.Fatal("flag beats env")
	}
	_ = cmd.Flags().Set("profile", "ghost")
	if _, err = ResolveProfile(f, cmd); err == nil {
		t.Fatal("unknown name must error, not fall through")
	}
}

func TestResolveProfile_NoneConfigured(t *testing.T) {
	f := &Factory{Config: core.CliConfig{ConfigVersion: 2}}
	t.Setenv("SHOPLAZZA_CLI_PROFILE", "")
	if _, err := ResolveProfile(f, newCmdWithProfileFlag()); err == nil {
		t.Fatal("must error loudly (no silent fallback)")
	}
}

// RES-06: --profile lookup is case-insensitive.
func TestResolveProfile_CaseInsensitive(t *testing.T) {
	f := &Factory{Config: core.CliConfig{ConfigVersion: 2,
		Profiles: []core.ProfileConfig{{Name: "prod-us"}}}}
	cmd := newCmdWithProfileFlag()
	_ = cmd.Flags().Set("profile", "Prod-US")
	if p, err := ResolveProfile(f, cmd); err != nil || p.Name != "prod-us" {
		t.Fatalf("case-insensitive lookup: %v %v", p, err)
	}
}
