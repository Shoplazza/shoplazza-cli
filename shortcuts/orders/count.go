package orders

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "orders",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count orders",
	Long:    "Return the total number of orders matching the filters, without fetching the rows.",
	Example: `  # How many paid orders since a date
  shoplazza orders +count --financial-status paid --since 2026-09-01

  # Count unfulfilled orders for one customer
  shoplazza orders +count --email a@b.com --fulfillment-status waiting`,
	Flags: append(append([]common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Fuzzy match on order number, customer name or email. Prefer --email for a known address."},
		{Name: "email", Type: common.FlagString, Description: "Filter by customer email (exact match)."},
		{Name: "customer-id", Type: common.FlagString, Description: "Filter by customer ID."},
	}, statusFlags()...),
		common.SinceFlag("when the order was placed", "placed_at"),
		common.UntilFlag("when the order was placed", "placed_at"),
	),
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "keyword", in.Flags.GetString("keyword"))
		cmdutil.AddSliceString(q, "customer_emails", in.Flags.GetString("email"))
		cmdutil.AddString(q, "customer_id", in.Flags.GetString("customer-id"))
		addStatusFilters(q, in)
		if err := common.AddTimeRange(q, in, "placed_at"); err != nil {
			return common.PlannedRequest{}, err
		}
		return PlanCount(q), nil
	},
}
