package products

import (
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "products",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count products",
	Flags: []common.Flag{
		{Name: "published", Type: common.FlagString, Description: "Filter by published status: published, unpublished, any (true/false also accepted).", Completions: []string{"published", "unpublished", "any"}},
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		ps, err := normalizePublishedStatus(in.Flags.GetString("published"))
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "published_status", ps)
		return PlanCount(q), nil
	},
}
