package orders

import (
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "orders",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search orders",
	Flags: []common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Order number, customer name, email, etc."},
		{Name: "status", Type: common.FlagString, Description: "Order status filter.",
			Completions: []string{"opened", "placed", "finished", "cancelled"}},
		{Name: "financial-status", Type: common.FlagString, Description: "Financial status filter.",
			Completions: []string{"waiting", "paying", "authorized", "partially_paid", "paid", "cancelled", "failed", "refunded"}},
		{Name: "fulfillment-status", Type: common.FlagString, Description: "Fulfillment status filter.",
			Completions: []string{"initialled", "waiting", "partially_shipped", "shipped", "finished", "cancelled", "returned"}},
		{Name: "customer-id", Type: common.FlagString, Description: "Filter by customer ID."},
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
		cmdutil.AddString(q, "query", in.Flags.GetString("keyword"))
		cmdutil.AddString(q, "status", in.Flags.GetString("status"))
		cmdutil.AddString(q, "financial_status", in.Flags.GetString("financial-status"))
		cmdutil.AddString(q, "fulfillment_status", in.Flags.GetString("fulfillment-status"))
		cmdutil.AddString(q, "customer_id", in.Flags.GetString("customer-id"))
		cmdutil.AddString(q, "placed_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "placed_at_max", in.Flags.GetString("until"))
		if pl > 0 {
			q["page_size"] = pl
		}
		return PlanList(q), nil
	},
}
