package orders

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "orders",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count orders",
	Flags: []common.Flag{
		{Name: "status", Type: common.FlagString, Completions: []string{"opened", "placed", "finished", "cancelled"}, Description: "Status filter."},
		{Name: "financial-status", Type: common.FlagString, Completions: []string{"waiting", "paying", "authorized", "partially_paid", "paid", "cancelled", "failed", "refunded"}, Description: "Financial status filter."},
		{Name: "fulfillment-status", Type: common.FlagString, Completions: []string{"initialled", "waiting", "partially_shipped", "shipped", "finished", "cancelled", "returned"}, Description: "Fulfillment status filter."},
		common.SinceFlag(),
		common.UntilFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "status", in.Flags.GetString("status"))
		cmdutil.AddString(q, "financial_status", in.Flags.GetString("financial-status"))
		cmdutil.AddString(q, "fulfillment_status", in.Flags.GetString("fulfillment-status"))
		cmdutil.AddString(q, "placed_at_min", in.Flags.GetString("since"))
		cmdutil.AddString(q, "placed_at_max", in.Flags.GetString("until"))
		return PlanCount(q), nil
	},
}
