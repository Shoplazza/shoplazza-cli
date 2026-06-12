package products

import (
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "products",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search products",
	Flags: []common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Filter by product title or SKU."},
		{Name: "published", Type: common.FlagString, Description: "Filter by published status (true|false).", Completions: []string{"true", "false"}},
		{Name: "vendor", Type: common.FlagString, Description: "Filter by vendor name."},
		{Name: "collection-id", Type: common.FlagString, Description: "Filter by collection ID."},
		{Name: "tags", Type: common.FlagStringSlice, Description: "Filter by tags (comma-separated)."},
		common.PageLimitFlag(),
		common.FieldsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		pl, err := common.GetValidatedPageLimit(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "title", in.Flags.GetString("keyword"))
		cmdutil.AddString(q, "published_status", in.Flags.GetString("published"))
		cmdutil.AddString(q, "vendor", in.Flags.GetString("vendor"))
		cmdutil.AddString(q, "collection_id", in.Flags.GetString("collection-id"))
		if tags := in.Flags.GetStringSlice("tags"); len(tags) > 0 {
			q["tags"] = strings.Join(tags, ",")
		}
		if pl > 0 {
			q["per_page"] = pl
		}
		if fields := in.Flags.GetStringSlice("fields"); len(fields) > 0 {
			q["fields"] = fields
		}
		return PlanList(q), nil
	},
}
