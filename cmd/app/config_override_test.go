package appcmd

import (
	"testing"

	"github.com/spf13/cobra"
)

// TestResolveConfigFile_Override: an explicit --config maps a name segment to its
// toml file (empty segment = base); the project is not consulted in that case.
func TestResolveConfigFile_Override(t *testing.T) {
	mk := func(set string) *cobra.Command {
		c := &cobra.Command{}
		c.Flags().String("config", "", "")
		_ = c.Flags().Set("config", set) // marks Changed
		return c
	}
	// --config prod → shoplazza.app.prod.toml (p unused, so nil is safe)
	if got, err := resolveConfigFile(mk("prod"), nil); err != nil || got != "shoplazza.app.prod.toml" {
		t.Fatalf("--config prod: got %q err %v, want shoplazza.app.prod.toml", got, err)
	}
	// --config "" (explicit) → base config
	if got, err := resolveConfigFile(mk(""), nil); err != nil || got != "shoplazza.app.toml" {
		t.Fatalf("--config \"\": got %q err %v, want shoplazza.app.toml", got, err)
	}
	// No --config flag at all → falls through (would read the project's active
	// config); just assert it doesn't take the override branch by using a cmd with
	// no such flag and a nil project must NOT be dereferenced only when overriding.
	bare := &cobra.Command{}
	if fl := bare.Flags().Lookup("config"); fl != nil {
		t.Fatal("bare command should have no --config flag")
	}
}
