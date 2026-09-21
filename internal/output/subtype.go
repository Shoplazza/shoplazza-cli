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
