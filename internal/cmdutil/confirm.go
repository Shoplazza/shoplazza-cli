package cmdutil

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// ConfirmDestructive asks a human to confirm an irreversible write before it
// runs. It is a human-only safety net: non-interactive callers (agents, pipes,
// CI) return nil immediately and proceed unchanged, so automation never blocks.
// Declining surfaces output.ErrCanceled (exit ExitCanceled). Callers must skip
// it in --dry-run, which previews rather than executes.
func ConfirmDestructive(f *Factory, title string) error {
	if !Interactive(f) {
		return nil
	}
	ok, err := interact.Confirm(title)
	if err != nil {
		return err
	}
	if !ok {
		return output.ErrCanceled()
	}
	return nil
}
