package customers

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "customers",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search customers",
	Long:    "Find customers by email, phone, or signup window. Returns one page — set --page-limit and follow has_more/cursor for the rest.",
	Example: `  # Find a customer by email
  shoplazza customers +search --email a@b.com

  # Customers created since a date, 50 per page
  shoplazza customers +search --since 2026-09-01 --page-limit 50`,
	Flags: []common.Flag{
		{Name: "email", Type: common.FlagString, Description: "Filter by email."},
		{Name: "phone", Type: common.FlagString, Description: "Filter by phone (matches the customer's primary contact)."},
		common.SinceFlag(),
		common.UntilFlag(),
		common.PageLimitFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		pl, err := common.GetValidatedPageLimit(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "email", in.Flags.GetString("email"))
		cmdutil.AddString(q, "contact", in.Flags.GetString("phone"))
		cmdutil.AddString(q, "created_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "created_at_max", in.Flags.GetString("until"))
		if pl > 0 {
			q["page_size"] = pl
		}
		return PlanList(q), nil
	},
}
