package products

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "products",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count products",
	Flags: []common.Flag{
		{Name: "published", Type: common.FlagString, Description: "Filter by published status.", Completions: []string{"true", "false"}},
		{Name: "vendor", Type: common.FlagString, Description: "Filter by vendor name."},
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "published_status", in.Flags.GetString("published"))
		cmdutil.AddString(q, "vendor", in.Flags.GetString("vendor"))
		return PlanCount(q), nil
	},
}
