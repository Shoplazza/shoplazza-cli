package theme_extension

import (
	"bytes"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
)

func TestTeGroupAndAlias(t *testing.T) {
	f := &cmdutil.Factory{}
	cmd := NewCmdThemeExtension(f)
	if cmd.Use != "theme-extension" {
		t.Fatalf("expected Use=theme-extension, got %q", cmd.Use)
	}
	// te is the sole alias
	found := false
	for _, a := range cmd.Aliases {
		if a == "te" {
			found = true
		}
	}
	if !found {
		t.Fatalf("expected alias te in %v", cmd.Aliases)
	}
	var buf bytes.Buffer
	cmd = NewCmdThemeExtension(f)
	cmd.SetOut(&buf)
	cmd.SetArgs([]string{"--help"})
	_ = cmd.Execute()
	for _, sub := range []string{"create", "serve", "build", "versions", "deploy", "list", "connect", "release"} {
		if !strings.Contains(buf.String(), sub) {
			t.Errorf("missing subcommand %q in te help", sub)
		}
	}
}

// TestTeStoreDomainHasShorthandS: every te subcommand that exposes
// --store-domain must also accept -s, matching the CLI-wide convention
// (cmd/auth uses -s for store-domain). -s must resolve to store-domain on
// each such command (also guards against a shorthand collision).
func TestTeStoreDomainHasShorthandS(t *testing.T) {
	f := &cmdutil.Factory{}
	root := NewCmdThemeExtension(f)
	checked := 0
	for _, sub := range root.Commands() {
		fl := sub.Flags().Lookup("store-domain")
		if fl == nil {
			continue
		}
		checked++
		if fl.Shorthand != "s" {
			t.Errorf("%s: --store-domain shorthand = %q, want \"s\"", sub.Name(), fl.Shorthand)
		}
		if sh := sub.Flags().ShorthandLookup("s"); sh == nil || sh.Name != "store-domain" {
			t.Errorf("%s: -s should resolve to --store-domain", sub.Name())
		}
	}
	if checked == 0 {
		t.Fatal("no te subcommand exposes --store-domain; test wired wrong")
	}
}

// TestTePrintingCommandsHaveNoJQ: --jq was removed from the te module commands
// (pipe to the `jq` tool instead; only the raw api / dynamic commands keep
// built-in --jq). No te subcommand should register a --jq flag.
func TestTePrintingCommandsHaveNoJQ(t *testing.T) {
	root := NewCmdThemeExtension(&cmdutil.Factory{})
	for _, sub := range root.Commands() {
		if fl := sub.Flags().Lookup("jq"); fl != nil {
			t.Errorf("%s: --jq must NOT be registered (removed from the te module)", sub.Name())
		}
	}
	// and it must NOT show in help (the user-visible contract)
	var buf bytes.Buffer
	list := NewCmdThemeExtension(&cmdutil.Factory{})
	list.SetOut(&buf)
	list.SetArgs([]string{"list", "--help"})
	_ = list.Execute()
	if strings.Contains(buf.String(), "--jq") {
		t.Errorf("te list --help should NOT mention --jq, got:\n%s", buf.String())
	}
}
