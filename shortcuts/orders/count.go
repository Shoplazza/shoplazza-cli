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
	Flags: []common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Fuzzy match on order number, customer name or email. Prefer --email for a known address."},
		{Name: "email", Type: common.FlagString, Description: "Filter by customer email (exact match)."},
		{Name: "customer-id", Type: common.FlagString, Description: "Filter by customer ID."},
		{Name: "status", Type: common.FlagString, Completions: []string{"opened", "placed", "finished", "cancelled"}, Description: "Status filter."},
		{Name: "financial-status", Type: common.FlagString, Completions: []string{"waiting", "paying", "authorized", "partially_paid", "paid", "cancelled", "failed", "refunded"}, Description: "Financial status filter."},
		{Name: "fulfillment-status", Type: common.FlagString, Completions: []string{"initialled", "waiting", "partially_shipped", "shipped", "finished", "cancelled", "returned"}, Description: "Fulfillment status filter."},
		common.SinceFlag(),
		common.UntilFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "keyword", in.Flags.GetString("keyword"))
		cmdutil.AddSliceString(q, "customer_emails", in.Flags.GetString("email"))
		cmdutil.AddString(q, "customer_id", in.Flags.GetString("customer-id"))
		cmdutil.AddString(q, "status", in.Flags.GetString("status"))
		cmdutil.AddString(q, "financial_status", in.Flags.GetString("financial-status"))
		cmdutil.AddString(q, "fulfillment_status", in.Flags.GetString("fulfillment-status"))
		cmdutil.AddString(q, "placed_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "placed_at_max", in.Flags.GetString("until"))
		return PlanCount(q), nil
	},
}
