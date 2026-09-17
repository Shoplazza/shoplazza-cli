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
