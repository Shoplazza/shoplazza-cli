package common

import "github.com/spf13/cobra"

// Shortcut declares a service-scoped +command mounted under a resource group.
// Consumed by common.Mount.
//
// Exactly one of Plan or Execute must be set (validated by ValidateShortcut):
//   - Plan: single-step shortcut wrapping one API endpoint.
//   - Execute: multi-step orchestration whose final request depends on data
//     from an earlier call (e.g., +ship needs line_item_ids from GET /orders/{id}).
type Shortcut struct {
	Service string
	Command string

	Use     string
	Short   string
	Long    string // optional extended help (`cmd --help` body); falls back to Short when empty
	Example string // optional worked invocations shown under "Examples:" in --help
	Args    cobra.PositionalArgs
	Flags   []Flag
	Plan    func(in PlanInput) (PlannedRequest, error)
	Execute ExecuteFunc

	// AuthFree marks a purely local command (no Shoplazza API calls) that must
	// run without login; Mount stamps cmdutil.AnnotationAuthFree so auth gates
	// skip it. Leave false for anything that touches the API.
	AuthFree bool

	// NotScannable marks a command blind CLI scans must skip (interactive,
	// long-running, or writes the local filesystem); Mount stamps
	// cmdutil.AnnotationNotScannable so the contract smoke suite discovers it.
	NotScannable bool

	// Local marks a command whose live result is a local artifact report (file
	// paths, counts), not an API response. The engine prints it via
	// output.PrintBody (raw body) instead of the {ok,data} success envelope.
	Local bool

	// Destructive marks an irreversible write (refund / cancel / delete /
	// unpublish). A human running it in an interactive terminal is asked to
	// confirm before it executes; agents and piped/CI runs are UNAFFECTED — they
	// proceed exactly as before, relying on --dry-run + skill discipline.
	// --dry-run always skips the prompt (it previews, it does not execute).
	Destructive   bool
	DestructiveIf func(FlagSet) string

	// ConfirmPrompt overrides the y/N question shown for a Destructive command.
	ConfirmPrompt string

	// ConfirmPhraseFlag, when set, upgrades the confirmation to "type this flag's
	// value to confirm" — the stronger gate for high-risk money ops (e.g. +refund
	// asks the user to type the order id). Falls back to y/N if the flag is empty.
	ConfirmPhraseFlag string

	// StoreTier lists the command in the module's store-operations help group
	// rather than the dev-tier one, for a shortcut that works on the store over
	// the API instead of on local files. Help rendering only.
	StoreTier bool
}
