package common

import (
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
	return fillRequiredWith(c, flags, cmdutil.Interactive(factory), promptFlag)
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

// confirmDestructive gates a Destructive command behind a confirmation in an
// interactive terminal. Non-interactive runs (agents, pipes, CI) return nil
// immediately — this is a human-only safety net and never blocks automation.
// Callers skip it in --dry-run (a preview does not execute).
func confirmDestructive(c *cobra.Command, s Shortcut, factory *cmdutil.Factory) error {
	return confirmDestructiveWith(c, s, cmdutil.Interactive(factory), interact.Confirm, interact.ConfirmTyped)
}

// confirmDestructiveWith is confirmDestructive with the gate and the confirm
// prompters injected, so every branch is testable without a real terminal. A
// type-the-value gate is used when the command names a ConfirmPhraseFlag and it
// is set (high-risk money ops); otherwise a plain y/N.
func confirmDestructiveWith(
	c *cobra.Command, s Shortcut, interactive bool,
	confirm func(title string) (bool, error),
	confirmTyped func(title, phrase string) (bool, error),
) error {
	if !interactive {
		return nil // human-only gate: agents/pipes/CI proceed unchanged
	}
	title := s.ConfirmPrompt
	if title == "" {
		title = "Run '" + s.Command + "'? This cannot be undone."
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

// promptFlag asks for one flag's value: a Select over its enum when it has
// Completions, otherwise a non-empty text Input. The title leads with the flag
// name so the user knows what they are answering.
func promptFlag(f Flag) (string, error) {
	title := "--" + f.Name
	if f.Description != "" {
		title += " — " + f.Description
	}
	if len(f.Completions) > 0 {
		return interact.Select(title, f.Completions)
	}
	return interact.Input(title, func(s string) error {
		if strings.TrimSpace(s) == "" {
			return fmt.Errorf("--%s is required", f.Name)
		}
		return nil
	})
}
