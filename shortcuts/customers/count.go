package customers

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "customers",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count customers",
	Long:    "Return the total number of customers matching the filters, without fetching the rows.",
	Example: `  # Total customers
  shoplazza customers +count

  # New customers since a date
  shoplazza customers +count --since 2026-09-01`,
	Flags: []common.Flag{
		{Name: "email", Type: common.FlagString, Description: "Filter by email."},
		{Name: "phone", Type: common.FlagString, Description: "Filter by phone (matches the customer's primary contact)."},
		common.SinceFlag(),
		common.UntilFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "email", in.Flags.GetString("email"))
		cmdutil.AddString(q, "contact", in.Flags.GetString("phone"))
		cmdutil.AddString(q, "created_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "created_at_max", in.Flags.GetString("until"))
		return PlanCount(q), nil
	},
}
