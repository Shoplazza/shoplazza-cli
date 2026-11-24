package common

import (
	"fmt"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// IDFlag returns the required --<resource>-id string flag of a single-resource
// shortcut, with --id kept working as a hidden alias.
func IDFlag(resource, description string) Flag {
	return Flag{
		Name:        resource + "-id",
		Aliases:     []string{"id"},
		Type:        FlagString,
		Required:    true,
		Description: description,
	}
}

// PageLimitFlag returns the standardized --page-limit int flag for pagination.
// All v202601 list shortcuts use this name; no --limit / --per-page aliases.
func PageLimitFlag() Flag {
	return Flag{
		Name:        "page-limit",
		Type:        FlagInt,
		Description: "Page size (1-250).",
	}
}

// ValidatePageLimit enforces the 1-250 bound advertised by PageLimitFlag.
// A value of 0 means unset and is allowed (the shortcut drops it from the query).
func ValidatePageLimit(pl int) error {
	if pl == 0 {
		return nil
	}
	if pl < 1 || pl > 250 {
		return output.ErrValidation("--page-limit must be between 1 and 250, got %d", pl)
	}
	return nil
}

// GetValidatedPageLimit reads --page-limit and runs ValidatePageLimit on it.
// Returns 0 when the flag is unset; the caller treats 0 as "no page_size".
func GetValidatedPageLimit(in PlanInput) (int, error) {
	pl := in.Flags.GetInt("page-limit")
	if err := ValidatePageLimit(pl); err != nil {
		return 0, err
	}
	return pl, nil
}

// SinceFlag returns --since, the lower bound written to <field>_min by
// AddTimeRange; subject says what the field times, e.g. "when the order was placed".
func SinceFlag(subject, field string) Flag {
	return Flag{
		Name:        "since",
		Type:        FlagString,
		Description: fmt.Sprintf("Lower bound on %s, inclusive (%s_min). Accepts %s.", subject, field, TimeFormats),
	}
}

// UntilFlag returns --until, the upper bound written to <field>_max by AddTimeRange.
func UntilFlag(subject, field string) Flag {
	return Flag{
		Name:        "until",
		Type:        FlagString,
		Description: fmt.Sprintf("Upper bound on %s, inclusive (%s_max). Accepts %s.", subject, field, TimeFormats),
	}
}

// StartTimeFlag returns --start, the activity's starts_at.
func StartTimeFlag() Flag {
	return Flag{
		Name:        "start",
		Type:        FlagString,
		Description: "Activity start time (starts_at). Accepts " + TimeFormats + ". Default: now.",
	}
}

// EndTimeFlag returns --end, the activity's ends_at; forever / -1 mean no expiry.
func EndTimeFlag() Flag {
	return Flag{
		Name:        "end",
		Type:        FlagString,
		Description: "Activity end time (ends_at). Accepts " + TimeFormats + "; or forever | -1 for no expiry. Default: forever.",
	}
}
