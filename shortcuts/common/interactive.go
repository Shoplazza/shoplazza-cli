package common

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// fillRequired resolves required flags the user left unset. In an interactive
// terminal it prompts for each — a Select when the flag has a fixed enum
// (Completions), a text Input otherwise — and writes the answer back onto the
// flag. Non-interactively it NEVER blocks: it collects the missing flags and
// returns one structured error naming them, so an agent/script keeps the same
// fail-fast contract, just with a clearer message and a hint.
//
// Gate-off (no TTY / CI / SHOPLAZZA_CLI_NO_INTERACTIVE) takes the validation
// path, so piped and automated runs stay byte-for-byte unchanged. This is the
// single engine-level hook that lets every shortcut fill its own required flags.
func fillRequired(c *cobra.Command, flags []Flag, factory *cmdutil.Factory) error {
	prompt := func(f Flag) (string, error) {
		return promptFlag(context.Background(), f, clientFetch(factory), interact.SelectFiltered)
	}
	return fillRequiredWith(c, flags, cmdutil.Interactive(factory), prompt)
}

// fillRequiredWith is fillRequired with the gate decision and the prompter
// injected, so both branches are testable without a real terminal.
func fillRequiredWith(c *cobra.Command, flags []Flag, interactive bool, prompt func(Flag) (string, error)) error {
	var missing []string
	for _, f := range flags {
		if !f.Required || c.Flags().Changed(f.Name) {
			continue
		}
		if !interactive {
			missing = append(missing, "--"+f.Name)
			continue
		}
		val, err := prompt(f)
		if err != nil {
			return err // output.ErrCanceled on esc/ctrl+c, else an internal render error
		}
		if err := c.Flags().Set(f.Name, val); err != nil {
			return output.ErrValidation("invalid --%s: %v", f.Name, err)
		}
	}
	if len(missing) > 0 {
		return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"required flag(s) not set: "+strings.Join(missing, ", "),
			"pass "+strings.Join(missing, " ")+", or run '"+c.CommandPath()+" --help' to see them")
	}
	return nil
}

func confirmTitle(s Shortcut, flags FlagSet) string {
	if s.DestructiveIf != nil {
		if title := s.DestructiveIf(flags); title != "" {
			return title
		}
	}
	if !s.Destructive {
		return ""
	}
	if s.ConfirmPrompt != "" {
		return s.ConfirmPrompt
	}
	return "Run '" + s.Command + "'? This cannot be undone."
}

// confirmDestructive gates a destructive invocation behind a confirmation in an
// interactive terminal. Non-interactive runs (agents, pipes, CI) return nil
// immediately — this is a human-only safety net and never blocks automation.
// Callers skip it in --dry-run (a preview does not execute).
func confirmDestructive(c *cobra.Command, s Shortcut, title string, factory *cmdutil.Factory) error {
	return confirmDestructiveWith(c, s, title, cmdutil.Interactive(factory), interact.Confirm, interact.ConfirmTyped)
}

// confirmDestructiveWith is confirmDestructive with the gate and the confirm
// prompters injected, so every branch is testable without a real terminal. A
// type-the-value gate is used when the command names a ConfirmPhraseFlag and it
// is set (high-risk money ops); otherwise a plain y/N.
func confirmDestructiveWith(
	c *cobra.Command, s Shortcut, title string, interactive bool,
	confirm func(title string) (bool, error),
	confirmTyped func(title, phrase string) (bool, error),
) error {
	if !interactive {
		return nil // human-only gate: agents/pipes/CI proceed unchanged
	}
	var (
		ok  bool
		err error
	)
	if phrase := phraseFlagValue(c, s); phrase != "" {
		ok, err = confirmTyped(title, phrase)
	} else {
		ok, err = confirm(title)
	}
	if err != nil {
		return err // output.ErrCanceled on esc/ctrl+c
	}
	if !ok {
		return output.ErrCanceled()
	}
	return nil
}

// phraseFlagValue returns the value the user must type to confirm, or "" when
// the command uses a plain y/N.
func phraseFlagValue(c *cobra.Command, s Shortcut) string {
	if s.ConfirmPhraseFlag == "" {
		return ""
	}
	v, _ := c.Flags().GetString(s.ConfirmPhraseFlag)
	return v
}

// promptFlag asks for one flag's value. In order of preference: a fuzzy
// resource picker when the flag declares one (fall back to text if the lookup
// is empty or fails), a Select over its enum when it has Completions, otherwise
// a non-empty text Input. The title leads with the flag name so the user knows
// what they are answering. fetch/choose are injected so both the picker path
// and the fallback are testable without a live server or a real terminal.
func promptFlag(ctx context.Context, f Flag, fetch fetchFunc, choose selectFunc) (string, error) {
	return promptFlagWith(ctx, f, fetch, choose, interact.Select, interact.Input)
}

// promptFlagWith is promptFlag with the enum-Select and text-Input leaves
// injected too (the picker's choose is already injected), so the full dispatch —
// picker → Completions Select → text Input, and the picker's empty/error
// fall-through — is testable without a real terminal.
func promptFlagWith(
	ctx context.Context, f Flag, fetch fetchFunc, choose selectFunc,
	selectEnum func(title string, options []string) (string, error),
	input func(title string, validate func(string) error) (string, error),
) (string, error) {
	title := "--" + f.Name
	if f.Description != "" {
		title += " — " + f.Description
	}
	if f.Picker != nil {
		v, ok, err := resolvePicker(ctx, f.Picker, title, fetch, choose)
		if err != nil {
			return "", err
		}
		if ok {
			return v, nil
		}
		// empty page or lookup failed → fall through to manual entry
	}
	if len(f.Completions) > 0 {
		return selectEnum(title, f.Completions)
	}
	return input(title, func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("--%s is required", f.Name)
		}
		return nil
	})
}
