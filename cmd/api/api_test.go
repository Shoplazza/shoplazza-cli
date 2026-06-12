package api

import (
	"io"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
)

func TestNewCmdAPI_HasRestSubcommand(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdAPI(f)
	if cmd.Use != "api" {
		t.Errorf("Use = %q, want api", cmd.Use)
	}
	found := false
	for _, sub := range cmd.Commands() {
		if sub.Use == "rest <method> <path>" {
			found = true
		}
	}
	if !found {
		t.Error("expected 'rest' subcommand under api")
	}
}

func TestNewCmdRest_RegistersFlags(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := newCmdRest(f)
	for _, name := range []string{"params", "data", "dry-run", "jq"} {
		if cmd.Flags().Lookup(name) == nil {
			t.Errorf("expected --%s flag on rest command", name)
		}
	}
}

// TestNewCmdRest_RunE_DryRun exercises buildRawRequest and the dry-run output path.
func TestNewCmdRest_RunE_DryRun(t *testing.T) {
	f := &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		Client:    client.New("http://localhost"),
	}
	cmd := newCmdRest(f)
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("dry-run", "true")
	if err := cmd.RunE(cmd, []string{"GET", "/orders"}); err != nil {
		t.Errorf("unexpected error in dry-run: %v", err)
	}
}

// TestNewCmdRest_RunE_DryRun_WithParams covers buildRawRequest with params.
func TestNewCmdRest_RunE_DryRun_WithParams(t *testing.T) {
	f := &cmdutil.Factory{
		IOStreams: cmdutil.IOStreams{In: strings.NewReader(""), Out: io.Discard, ErrOut: io.Discard},
		Client:    client.New("http://localhost"),
	}
	cmd := newCmdRest(f)
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("dry-run", "true")
	_ = cmd.Flags().Set("params", `{"limit":10}`)
	if err := cmd.RunE(cmd, []string{"GET", "/products"}); err != nil {
		t.Errorf("unexpected error in dry-run with params: %v", err)
	}
}
