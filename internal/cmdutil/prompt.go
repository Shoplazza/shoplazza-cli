package cmdutil

import (
	"context"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// PromptField describes one value a plain (non-shortcut) cobra command needs
// resolved before it runs. It is the cmd/ counterpart of the shortcut engine's
// interactive fill: shortcuts get this for free, individual commands opt in by
// listing their required inputs.
//
// When the flag is unset and the terminal is interactive the user is prompted —
// a fuzzy SelectFiltered via Picker (falling back to text if the lookup is
// empty or fails), a Select over Choices, or a text Input. Non-interactively an
// unset required field is collected and reported as one structured error, so
// agents/pipes/CI keep the same fail-fast contract they get from shortcuts.
type PromptField struct {
	Flag    string   // cobra flag to read and write back
	Title   string   // prompt title; defaults to "--"+Flag
	Choices []string // fixed enum → Select
	// Picker supplies live choices for a fuzzy SelectFiltered. It may read
	// sibling flags off cmd (e.g. a version list keyed by an already-resolved
	// --extension-id), so order fields so dependencies resolve first.
	Picker func(ctx context.Context, cmd *cobra.Command, f *Factory) ([]interact.Option, error)
	// Optional keeps the field from being reported as missing non-interactively;
	// it is still offered to a human. Use for inputs that are only conditionally
	// required and validated downstream.
	Optional bool
	// When, if set, resolves the field only when it returns true (e.g. a theme
	// subtype that applies solely to theme extensions).
	When func(cmd *cobra.Command) bool
}

// ResolveFlags fills each unset field per PromptField's rules. See PromptField.
func ResolveFlags(cmd *cobra.Command, f *Factory, fields ...PromptField) error {
	resolve := func(fld PromptField) (string, error) {
		return resolveField(cmd.Context(), cmd, f, fld)
	}
	return resolveFlagsWith(cmd, fields, Interactive(f), resolve)
}

// resolveFlagsWith is ResolveFlags with the gate decision and the per-field
// resolver injected, so both branches are testable without a real terminal.
func resolveFlagsWith(cmd *cobra.Command, fields []PromptField, interactive bool, resolve func(PromptField) (string, error)) error {
	var missing []string
	for _, fld := range fields {
		if fld.When != nil && !fld.When(cmd) {
			continue
		}
		if cmd.Flags().Changed(fld.Flag) {
			continue
		}
		if !interactive {
			if !fld.Optional {
				missing = append(missing, "--"+fld.Flag)
			}
			continue
		}
		val, err := resolve(fld)
		if err != nil {
			return err // output.ErrCanceled on esc/ctrl+c, else a render error
		}
		if val == "" {
			continue // nothing chosen/typed; leave unset for downstream validation
		}
		if err := cmd.Flags().Set(fld.Flag, val); err != nil {
			return output.ErrValidation("invalid --%s: %v", fld.Flag, err)
		}
	}
	if len(missing) > 0 {
		return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"required flag(s) not set: "+strings.Join(missing, ", "),
			"pass "+strings.Join(missing, " ")+", or run '"+cmd.CommandPath()+" --help' to see them")
	}
	return nil
}

// resolveField prompts for one field: a fuzzy picker when it declares one (fall
// back to text if the lookup is empty or errors), a Select over its enum, else
// a non-empty text Input.
func resolveField(ctx context.Context, cmd *cobra.Command, f *Factory, fld PromptField) (string, error) {
	return resolveFieldWith(ctx, cmd, f, fld, interact.SelectFiltered, interact.Select, interact.Input)
}

// resolveFieldWith is resolveField with the three leaf prompters injected, so
// the dispatch (picker → enum Select → text Input) and the picker fall-through
// are testable without a real terminal.
func resolveFieldWith(
	ctx context.Context, cmd *cobra.Command, f *Factory, fld PromptField,
	selectFiltered func(title string, options []interact.Option) (string, error),
	selectEnum func(title string, options []string) (string, error),
	input func(title string, validate func(string) error) (string, error),
) (string, error) {
	title := fld.Title
	if title == "" {
		title = "--" + fld.Flag
	}
	if fld.Picker != nil {
		if opts, err := fld.Picker(ctx, cmd, f); err == nil && len(opts) > 0 {
			return selectFiltered(title, opts)
		}
		// empty page or lookup failure → fall through to manual entry
	}
	if len(fld.Choices) > 0 {
		return selectEnum(title, fld.Choices)
	}
	return input(title, func(s string) error {
		// Optional fields may be left blank (leaving the flag unset); only a
		// required field rejects an empty answer.
		if !fld.Optional && strings.TrimSpace(s) == "" {
			return fmt.Errorf("--%s is required", fld.Flag)
		}
		return nil
	})
}
