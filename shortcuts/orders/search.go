package orders

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "orders",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search orders",
	Long:    "Search orders by keyword, email, status, or time window. Returns one page — set --page-limit and follow has_more/cursor for the rest.",
	Example: `  # Paid orders since a date, 50 per page
  shoplazza orders +search --financial-status paid --since 2026-09-01 --page-limit 50

  # By customer email, print only order numbers
  shoplazza orders +search --email a@b.com --jq '.data.orders[].number'`,
	Flags: append(append([]common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Fuzzy match on order number, customer name or email. Prefer --email for a known address."},
		{Name: "email", Type: common.FlagString, Description: "Filter by customer email (exact match)."},
	}, statusFlags()...), []common.Flag{
		{Name: "customer-id", Type: common.FlagString, Description: "Filter by customer ID."},
		common.SinceFlag("when the order was placed", "placed_at"),
		common.UntilFlag("when the order was placed", "placed_at"),
		common.PageLimitFlag(),
	}...),
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		pl, err := common.GetValidatedPageLimit(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "keyword", in.Flags.GetString("keyword"))
		cmdutil.AddSliceString(q, "customer_emails", in.Flags.GetString("email"))
		addStatusFilters(q, in)
		cmdutil.AddString(q, "customer_id", in.Flags.GetString("customer-id"))
		if err := common.AddTimeRange(q, in, "placed_at"); err != nil {
			return common.PlannedRequest{}, err
		}
		if pl > 0 {
			q["page_size"] = pl
		}
		return PlanList(q), nil
	},
}
