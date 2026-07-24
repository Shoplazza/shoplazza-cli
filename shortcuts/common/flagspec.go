package common

import "github.com/Shoplazza/shoplazza-cli/v2/internal/output"

// IDFlag returns a required --id string flag with the given description.
// Used for single-resource shortcuts (+get, +delete, +publish, +cancel, etc.).
func IDFlag(description string) Flag {
	return Flag{
		Name:        "id",
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

// SinceFlag returns the standardized --since string flag (ISO date).
// Maps to the API's created_at_min / placed_at_min depending on the endpoint.
func SinceFlag() Flag {
	return Flag{
		Name:        "since",
		Type:        FlagString,
		Description: "Lower time bound (ISO date or unix ts).",
	}
}

// UntilFlag returns the standardized --until string flag (ISO date).
func UntilFlag() Flag {
	return Flag{
		Name:        "until",
		Type:        FlagString,
		Description: "Upper time bound (ISO date or unix ts).",
	}
}

// FieldsFlag returns the standardized --fields []string flag for response field projection.
func FieldsFlag() Flag {
	return Flag{
		Name:        "fields",
		Type:        FlagStringSlice,
		Description: "Response fields to include (comma-separated).",
	}
}

// StartTimeFlag returns the standardized --start string flag for
// activity/campaign start. Values are parsed by ParseTime.
func StartTimeFlag() Flag {
	return Flag{
		Name:        "start",
		Type:        FlagString,
		Description: "Start time (now | +Nh/+Nd/+Nw | YYYY-MM-DDTHH:MM:SS | unix-seconds; UTC; default: now).",
	}
}

// EndTimeFlag returns the standardized --end string flag for
// activity/campaign end. Values are parsed by ParseTime; forever / -1
// mean no expiry.
func EndTimeFlag() Flag {
	return Flag{
		Name:        "end",
		Type:        FlagString,
		Description: "End time (now | +Nh/+Nd/+Nw | YYYY-MM-DDTHH:MM:SS | unix-seconds | forever | -1; UTC; default: -1 = no expiry).",
	}
}
