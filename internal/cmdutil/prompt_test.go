package cmdutil

import (
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func promptCmd(flags ...string) *cobra.Command {
	c := &cobra.Command{Use: "demo"}
	for _, n := range flags {
		c.Flags().String(n, "", "")
	}
	return c
}

func TestResolveFlags_NonInteractive_NamesMissingRequired(t *testing.T) {
	c := promptCmd("extension-id", "version", "note")
	fields := []PromptField{
		{Flag: "extension-id"},
		{Flag: "version"},
		{Flag: "note", Optional: true},
	}
	err := resolveFlagsWith(c, fields, false, func(PromptField) (string, error) {
		t.Fatal("non-interactive must never prompt")
		return "", nil
	})
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want *output.ExitError, got %T", err)
	}
	for _, want := range []string{"--extension-id", "--version"} {
		if !strings.Contains(ee.Detail.Message, want) {
			t.Errorf("message %q should name %s", ee.Detail.Message, want)
		}
	}
	if strings.Contains(ee.Detail.Message, "--note") {
		t.Errorf("Optional field must not be reported missing: %q", ee.Detail.Message)
	}
}

func TestResolveFlags_NonInteractive_AllSet_OK(t *testing.T) {
	c := promptCmd("extension-id", "version")
	_ = c.Flags().Set("extension-id", "E1")
	_ = c.Flags().Set("version", "1.0")
	fields := []PromptField{{Flag: "extension-id"}, {Flag: "version"}}
	if err := resolveFlagsWith(c, fields, false, func(PromptField) (string, error) { return "", nil }); err != nil {
		t.Errorf("all required set → no error, got %v", err)
	}
}

func TestResolveFlags_Interactive_PromptsAndWritesBack(t *testing.T) {
	c := promptCmd("extension-id", "version")
	answers := map[string]string{"extension-id": "E9", "version": "2.0"}
	resolve := func(f PromptField) (string, error) { return answers[f.Flag], nil }
	fields := []PromptField{{Flag: "extension-id"}, {Flag: "version"}}
	if err := resolveFlagsWith(c, fields, true, resolve); err != nil {
		t.Fatal(err)
	}
	for name, want := range answers {
		if got, _ := c.Flags().GetString(name); got != want {
			t.Errorf("%s = %q, want %q (resolved value must be written onto the flag)", name, got, want)
		}
	}
}

func TestResolveFlags_Interactive_SkipsAlreadySet(t *testing.T) {
	c := promptCmd("extension-id", "version")
	_ = c.Flags().Set("extension-id", "given")
	var prompted []string
	resolve := func(f PromptField) (string, error) { prompted = append(prompted, f.Flag); return "x", nil }
	fields := []PromptField{{Flag: "extension-id"}, {Flag: "version"}}
	if err := resolveFlagsWith(c, fields, true, resolve); err != nil {
		t.Fatal(err)
	}
	if len(prompted) != 1 || prompted[0] != "version" {
		t.Errorf("only the unset flag should be prompted, got %v", prompted)
	}
}

func TestResolveFlags_WhenGate(t *testing.T) {
	c := promptCmd("type", "theme-type")
	_ = c.Flags().Set("type", "checkout") // not a theme → theme-type must be skipped
	var prompted []string
	resolve := func(f PromptField) (string, error) { prompted = append(prompted, f.Flag); return "x", nil }
	fields := []PromptField{
		{Flag: "theme-type", When: func(cmd *cobra.Command) bool { t, _ := cmd.Flags().GetString("type"); return t == "theme" }},
	}
	if err := resolveFlagsWith(c, fields, true, resolve); err != nil {
		t.Fatal(err)
	}
	if len(prompted) != 0 {
		t.Errorf("When=false must skip the field entirely, prompted %v", prompted)
	}
}

func TestResolveFlags_Interactive_CancelPropagates(t *testing.T) {
	c := promptCmd("name")
	resolve := func(PromptField) (string, error) { return "", output.ErrCanceled() }
	fields := []PromptField{{Flag: "name"}}
	if err := resolveFlagsWith(c, fields, true, resolve); err == nil {
		t.Error("a canceled prompt must propagate, not silently proceed")
	}
}
