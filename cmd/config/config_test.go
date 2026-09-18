package configcmd

import (
	"bytes"
	"path/filepath"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/core"
)

// isolateConfig points DefaultConfigPath at a temp dir on both macOS
// ($HOME/Library/Application Support) and Linux ($XDG_CONFIG_HOME).
func isolateConfig(t *testing.T) {
	t.Helper()
	dir := t.TempDir()
	t.Setenv("HOME", dir)
	t.Setenv("XDG_CONFIG_HOME", filepath.Join(dir, ".config"))
}

func run(t *testing.T, cmd *cobra.Command, args []string) error {
	t.Helper()
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	return cmd.RunE(cmd, args)
}

// TestConfigFormat_SetPersists: `config format pretty` validates and saves; a
// later load sees it.
func TestConfigFormat_SetPersists(t *testing.T) {
	isolateConfig(t)
	f := &cmdutil.Factory{}

	if err := run(t, newCmdConfigFormat(f), []string{"pretty"}); err != nil {
		t.Fatalf("set: %v", err)
	}
	path, err := core.DefaultConfigPath()
	if err != nil {
		t.Fatalf("config path: %v", err)
	}
	cfg, err := core.LoadConfig(path)
	if err != nil {
		t.Fatalf("load: %v", err)
	}
	if cfg.Format != "pretty" {
		t.Fatalf("saved format = %q, want pretty", cfg.Format)
	}
}

// TestConfigFormat_RejectsInvalid: an unknown format errors and writes nothing.
func TestConfigFormat_RejectsInvalid(t *testing.T) {
	isolateConfig(t)
	f := &cmdutil.Factory{}

	if err := run(t, newCmdConfigFormat(f), []string{"bogus"}); err == nil {
		t.Fatal("invalid format must error")
	}
	path, _ := core.DefaultConfigPath()
	cfg, _ := core.LoadConfig(path)
	if cfg.Format != "" {
		t.Fatalf("nothing should have been saved, got %q", cfg.Format)
	}
}
