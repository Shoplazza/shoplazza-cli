package interact

import (
	"fmt"
	"strings"

	"github.com/charmbracelet/huh"
)

// Input prompts for a single line of text on the terminal and returns it
// trimmed. title is the question; validate (nil-ok) rejects bad input on submit
// (wrapped in OnlyOnSubmit so it fires on enter, not on focus). Canceling with
// esc/ctrl+c surfaces output.ErrCanceled via Run. The UI draws on stderr, so
// stdout stays clean for the result envelope.
func Input(title string, validate func(string) error) (string, error) {
	var v string
	field := huh.NewInput().Title(title).Value(&v)
	if validate != nil {
		field = field.Validate(OnlyOnSubmit(validate))
	}
	err := Run(func() *huh.Form { return NewForm(huh.NewGroup(field)) })
	return strings.TrimSpace(v), err
}

// Select prompts the user to choose one of options on the terminal and returns
// the choice. Canceling surfaces output.ErrCanceled via Run.
func Select(title string, options []string) (string, error) {
	var v string
	opts := make([]huh.Option[string], len(options))
	for i, o := range options {
		opts[i] = huh.NewOption(o, o)
	}
	field := huh.NewSelect[string]().Title(title).Options(opts...).Value(&v)
	err := Run(func() *huh.Form { return NewForm(huh.NewGroup(field)) })
	return v, err
}

// Confirm asks a yes/no question on the terminal, defaulting to No. It returns
// false when the user declines. Canceling (esc/ctrl+c) surfaces
// output.ErrCanceled via Run. Use for ordinary destructive confirmations.
func Confirm(title string) (bool, error) {
	var ok bool
	field := huh.NewConfirm().Title(title).Affirmative("Yes").Negative("No").Value(&ok)
	err := Run(func() *huh.Form { return NewForm(huh.NewGroup(field)) })
	return ok, err
}

// ConfirmTyped requires the user to type phrase exactly before proceeding — the
// stronger gate for high-risk, irreversible actions (e.g. a refund). It returns
// true only on an exact match; a mismatch keeps the field (the user can fix it
// or press esc to cancel, which surfaces output.ErrCanceled).
func ConfirmTyped(title, phrase string) (bool, error) {
	var typed string
	field := huh.NewInput().
		Title(title).
		Description(fmt.Sprintf("Type %s to confirm.", phrase)).
		Value(&typed).
		Validate(OnlyOnSubmit(func(s string) error {
			if strings.TrimSpace(s) != phrase {
				return fmt.Errorf("does not match %q", phrase)
			}
			return nil
		}))
	err := Run(func() *huh.Form { return NewForm(huh.NewGroup(field)) })
	return strings.TrimSpace(typed) == phrase, err
}
