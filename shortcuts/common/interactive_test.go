package common

import (
	"bytes"
	"errors"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// nonInteractiveFactory returns a Factory whose IO streams are not terminals, so
// cmdutil.Interactive is false and fillRequired takes the fail-fast path.
func nonInteractiveFactory() *cmdutil.Factory {
	return &cmdutil.Factory{IOStreams: cmdutil.IOStreams{
		In:     &bytes.Buffer{},
		Out:    &bytes.Buffer{},
		ErrOut: &bytes.Buffer{},
	}}
}

func cmdWithFlags(names ...string) *cobra.Command {
	c := &cobra.Command{Use: "demo"}
	for _, n := range names {
		c.Flags().String(n, "", "")
	}
	return c
}

func cmdWithBoolFlags(names ...string) *cobra.Command {
	c := &cobra.Command{Use: "demo"}
	for _, n := range names {
		c.Flags().Bool(n, false, "")
	}
	return c
}

func TestFillRequired_NonInteractive_NamesMissing(t *testing.T) {
	c := cmdWithFlags("title", "price", "note")
	flags := []Flag{
		{Name: "title", Required: true},
		{Name: "price", Required: true},
		{Name: "note", Required: false},
	}
	err := fillRequired(c, flags, nonInteractiveFactory())
	if err == nil {
		t.Fatal("non-interactive with missing required flags must error")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("want *output.ExitError, got %T", err)
	}
	msg := ee.Detail.Message
	hint := ee.Detail.Hint
	// Names the two missing required flags, not the optional one.
	for _, want := range []string{"--title", "--price"} {
		if !strings.Contains(msg, want) {
			t.Errorf("message %q should name %s", msg, want)
		}
	}
	if strings.Contains(msg, "--note") {
		t.Errorf("optional flag must not be named: %q", msg)
	}
	if !strings.Contains(hint, "--title") || hint == "" {
		t.Errorf("hint should tell the user how to pass them: %q", hint)
	}
}

func TestFillRequired_NonInteractive_AllSet_OK(t *testing.T) {
	c := cmdWithFlags("title", "price")
	_ = c.Flags().Set("title", "T-shirt")
	_ = c.Flags().Set("price", "29.99")
	flags := []Flag{{Name: "title", Required: true}, {Name: "price", Required: true}}
	if err := fillRequired(c, flags, nonInteractiveFactory()); err != nil {
		t.Errorf("all required flags set → no error, got %v", err)
	}
}

func TestFillRequired_NonInteractive_NoRequired_OK(t *testing.T) {
	c := cmdWithFlags("note")
	flags := []Flag{{Name: "note", Required: false}}
	if err := fillRequired(c, flags, nonInteractiveFactory()); err != nil {
		t.Errorf("no required flags → no error, got %v", err)
	}
}

// Interactive path (prompter injected so no real terminal is needed).

func TestFillRequired_Interactive_PromptsAndFills(t *testing.T) {
	c := cmdWithFlags("title", "price", "type")
	answers := map[string]string{"title": "T-shirt", "price": "29.99", "type": "basic"}
	prompt := func(f Flag) (string, error) { return answers[f.Name], nil }
	flags := []Flag{
		{Name: "title", Required: true},
		{Name: "price", Required: true},
		{Name: "type", Required: true, Completions: []string{"basic", "embed"}},
	}
	if err := fillRequiredWith(c, flags, true, prompt); err != nil {
		t.Fatalf("interactive fill: %v", err)
	}
	for name, want := range answers {
		if got, _ := c.Flags().GetString(name); got != want {
			t.Errorf("%s = %q, want %q (prompted value must be written back onto the flag)", name, got, want)
		}
	}
}

func TestFillRequired_Interactive_SkipsAlreadySet(t *testing.T) {
	c := cmdWithFlags("title", "price")
	_ = c.Flags().Set("title", "given")
	var promptedFor []string
	prompt := func(f Flag) (string, error) { promptedFor = append(promptedFor, f.Name); return "x", nil }
	flags := []Flag{{Name: "title", Required: true}, {Name: "price", Required: true}}
	if err := fillRequiredWith(c, flags, true, prompt); err != nil {
		t.Fatal(err)
	}
	if len(promptedFor) != 1 || promptedFor[0] != "price" {
		t.Errorf("only the unset required flag should be prompted, got %v", promptedFor)
	}
}

func TestFillRequired_Interactive_CancelPropagates(t *testing.T) {
	c := cmdWithFlags("title")
	prompt := func(_ Flag) (string, error) { return "", output.ErrCanceled() }
	flags := []Flag{{Name: "title", Required: true}}
	if err := fillRequiredWith(c, flags, true, prompt); err == nil {
		t.Error("a canceled prompt must propagate, not silently proceed")
	}
}

// Destructive confirmation (human-only; prompters injected).

func TestConfirmDestructive_NonInteractive_Proceeds(t *testing.T) {
	c := cmdWithFlags("id")
	called := false
	confirm := func(string) (bool, error) { called = true; return false, nil }
	s := Shortcut{Command: "+refund", Destructive: true}
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), false, confirm, nil); err != nil {
		t.Errorf("non-interactive must proceed unchanged, got %v", err)
	}
	if called {
		t.Error("non-interactive must never prompt (agents must not block)")
	}
}

func TestConfirmDestructive_Interactive_YesProceeds_NoCancels(t *testing.T) {
	c := cmdWithFlags("id")
	s := Shortcut{Command: "+unpublish", Destructive: true}
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), true, func(string) (bool, error) { return true, nil }, nil); err != nil {
		t.Errorf("confirmed → proceed, got %v", err)
	}
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), true, func(string) (bool, error) { return false, nil }, nil); err == nil {
		t.Error("declining must cancel, not proceed")
	}
}

func TestConfirmDestructive_TypedGate_RequiresPhraseFlagValue(t *testing.T) {
	c := cmdWithFlags("order-id")
	_ = c.Flags().Set("order-id", "O123")
	var gotPhrase string
	confirmTyped := func(_, phrase string) (bool, error) { gotPhrase = phrase; return true, nil }
	ynCalled := false
	confirm := func(string) (bool, error) { ynCalled = true; return true, nil }
	s := Shortcut{Command: "+refund", Destructive: true, ConfirmPrompt: "Refund?", ConfirmPhraseFlag: "order-id"}
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), true, confirm, confirmTyped); err != nil {
		t.Fatal(err)
	}
	if ynCalled {
		t.Error("with a phrase flag set, use the type-to-confirm gate, not y/N")
	}
	if gotPhrase != "O123" {
		t.Errorf("high-risk gate must require typing the order id, phrase = %q", gotPhrase)
	}
}

func TestConfirmDestructive_TypedGate_EmptyPhraseFallsBackToYN(t *testing.T) {
	c := cmdWithFlags("order-id") // left unset → empty
	ynCalled, typedCalled := false, false
	confirm := func(string) (bool, error) { ynCalled = true; return true, nil }
	confirmTyped := func(_, _ string) (bool, error) { typedCalled = true; return true, nil }
	s := Shortcut{Command: "+refund", Destructive: true, ConfirmPhraseFlag: "order-id"}
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), true, confirm, confirmTyped); err != nil {
		t.Fatal(err)
	}
	if typedCalled || !ynCalled {
		t.Error("an empty phrase flag must fall back to y/N")
	}
}

// Per-invocation confirmation (DestructiveIf): a command that is safe by
// default and only irreversible with certain flags.

func TestConfirmTitle_DestructiveIf_SilentUntilFlagSet(t *testing.T) {
	s := Shortcut{Command: "+edit", DestructiveIf: func(f FlagSet) string {
		if f.GetBool("promote") {
			return "Save onto live theme's draft?"
		}
		return ""
	}}
	c := cmdWithBoolFlags("promote")
	if title := confirmTitle(s, NewCobraFlagSet(c)); title != "" {
		t.Errorf("a safe invocation must not confirm, got %q", title)
	}
	_ = c.Flags().Set("promote", "true")
	if title := confirmTitle(s, NewCobraFlagSet(c)); title != "Save onto live theme's draft?" {
		t.Errorf("the hook owns its wording, got %q", title)
	}
}

func TestConfirmTitle_StaticGatesUnchanged(t *testing.T) {
	flags := NewCobraFlagSet(cmdWithFlags("id"))
	if title := confirmTitle(Shortcut{Command: "+get"}, flags); title != "" {
		t.Errorf("a plain command must not confirm, got %q", title)
	}
	s := Shortcut{Command: "+refund", Destructive: true, ConfirmPrompt: "Refund?"}
	if title := confirmTitle(s, flags); title != "Refund?" {
		t.Errorf("ConfirmPrompt still wins, got %q", title)
	}
	want := "Run '+unpublish'? This cannot be undone."
	if title := confirmTitle(Shortcut{Command: "+unpublish", Destructive: true}, flags); title != want {
		t.Errorf("fallback = %q, want %q", title, want)
	}
}

func TestConfirmDestructive_DestructiveIf_NonInteractive_NeverPrompts(t *testing.T) {
	c := cmdWithBoolFlags("promote")
	_ = c.Flags().Set("promote", "true")
	s := Shortcut{Command: "+edit", DestructiveIf: func(FlagSet) string { return "Go live?" }}
	called := false
	confirm := func(string) (bool, error) { called = true; return false, nil }
	if err := confirmDestructiveWith(c, s, confirmTitle(s, NewCobraFlagSet(c)), false, confirm, nil); err != nil {
		t.Errorf("agents/pipes/CI must proceed unchanged, got %v", err)
	}
	if called {
		t.Error("the per-invocation gate must never prompt non-interactively")
	}
}
