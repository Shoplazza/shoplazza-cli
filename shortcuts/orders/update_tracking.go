package orders

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

var updateTrackingShortcut = common.Shortcut{
	Service: "orders",
	Command: "+update-tracking",
	Use:     "+update-tracking --order-id <id> --fulfillment-id <id> --tracking <no>",
	Short:   "Update tracking info on an existing fulfillment",
	Flags: []common.Flag{
		{Name: "order-id", Type: common.FlagString, Required: true, Description: "Order ID."},
		{Name: "fulfillment-id", Type: common.FlagString, Required: true, Description: "Fulfillment ID."},
		{Name: "tracking", Type: common.FlagString, Required: true, Description: "New tracking number."},
		{Name: "company", Type: common.FlagString, Description: "Carrier company name."},
		{Name: "tracking-url", Type: common.FlagString, Description: "Custom tracking URL."},
		{Name: "notify", Type: common.FlagBool, Description: "Notify customer."},
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		fulfillment := map[string]any{
			"tracking_number": in.Flags.GetString("tracking"),
		}
		cmdutil.AddString(fulfillment, "tracking_company", in.Flags.GetString("company"))
		cmdutil.AddString(fulfillment, "tracking_url", in.Flags.GetString("tracking-url"))
		if in.Flags.GetBool("notify") {
			fulfillment["send_email"] = true
		}
		return PlanUpdateFulfillment(
			in.Flags.GetString("order-id"),
			in.Flags.GetString("fulfillment-id"),
			map[string]any{"fulfillment": fulfillment},
		), nil
	},
}
