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
}
