package output

import "strings"

// Stable error subtypes. Type (api/validation/...) is the coarse class agents
// branch on for exit codes; Subtype is a stable machine-branchable id within a
// class, so an agent can recover precisely without matching message text.
// Values are contract — rename is a breaking change.
const (
	SubtypeUnknownCommand      = "unknown_command"
	SubtypeUnknownFlag         = "unknown_flag"
	SubtypeMissingRequiredFlag = "missing_required_flag"
	SubtypeInvalidFlagValue    = "invalid_flag_value"
	SubtypeInvalidArgument     = "invalid_argument"
	SubtypeUsage               = "usage_error" // generic cobra usage fallback

	// Spec-leaf --params / --data checks against the command's metadata.
	SubtypeUnknownParam         = "unknown_param"          // --params key the command does not declare
	SubtypeMisplacedParam       = "misplaced_param"        // --params key that is a request-body field
	SubtypeMissingRequiredField = "missing_required_field" // required top-level body field absent from --data
	SubtypeMissingRequiredParam = "missing_required_param" // required path/query key absent from --params, empty, or a placeholder

	SubtypeJQAfterRequest = "jq_failed_after_request" // the request succeeded; --jq failed on its response

	SubtypeInvalidToken = "invalid_token" // the store answered 400 "Unauthorized" to the access token
	SubtypeMissingScope = "missing_scope" // the store answered 403 with no reason
)

// ClassifyUsageError converts a cobra/pflag usage error (unknown command,
// unknown flag, missing required flag, bad argument) into a validation-class
// ExitError carrying a stable subtype and a next-step hint, so the commonest
// agent mistakes emit a parseable envelope instead of plain "Error: ..." text.
func ClassifyUsageError(err error) *ExitError {
	msg := strings.TrimSpace(err.Error())
	subtype := SubtypeUsage
	hint := "run the command with --help to see valid usage"

	switch {
	case strings.HasPrefix(msg, "unknown command"):
		subtype = SubtypeUnknownCommand
		hint = "run 'shoplazza --help' to list available commands"
	case strings.HasPrefix(msg, "unknown flag"), strings.HasPrefix(msg, "unknown shorthand flag"):
		subtype = SubtypeUnknownFlag
		hint = "run the command with --help to list valid flags"
	case strings.Contains(msg, "required flag"):
		subtype = SubtypeMissingRequiredFlag
		hint = "run the command with --help to see required flags"
	case strings.HasPrefix(msg, "invalid argument"),
		strings.HasPrefix(msg, "invalid value"),
		strings.HasPrefix(msg, "flag needs an argument"):
		subtype = SubtypeInvalidFlagValue
	case strings.HasPrefix(msg, "accepts"),
		strings.Contains(msg, "arg(s), received"),
		strings.HasPrefix(msg, "requires"):
		subtype = SubtypeInvalidArgument
	}

	e := ErrValidation("%s", msg)
	e.Detail.Subtype = subtype
	e.Detail.Hint = hint
	return e
}
