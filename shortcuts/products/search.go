package products

import (
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "products",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search products",
	Flags: []common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Filter by product title."},
		{Name: "published", Type: common.FlagString, Description: "Filter by published status: published, unpublished, any (true/false also accepted).", Completions: []string{"published", "unpublished", "any"}},
		{Name: "vendor", Type: common.FlagString, Description: "Filter by vendor name (exact match)."},
		{Name: "collection-id", Type: common.FlagString, Description: "Filter by collection ID."},
		common.PageLimitFlag(),
		common.FieldsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		pl, err := common.GetValidatedPageLimit(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		ps, err := normalizePublishedStatus(in.Flags.GetString("published"))
		if err != nil {
			return common.PlannedRequest{}, err
		}
		q := map[string]any{}
		cmdutil.AddString(q, "title", in.Flags.GetString("keyword"))
		cmdutil.AddString(q, "published_status", ps)
		if v := strings.TrimSpace(in.Flags.GetString("vendor")); v != "" {
			q["vendors"] = []string{v} // API param is `vendors` (array), not `vendor`.
		}
		cmdutil.AddString(q, "collection_id", in.Flags.GetString("collection-id"))
		if pl > 0 {
			q["per_page"] = pl
		}
		if fields := in.Flags.GetStringSlice("fields"); len(fields) > 0 {
			q["fields"] = fields
		}
		return PlanList(q), nil
	},
}

// normalizePublishedStatus maps --published to the API's published_status enum
// (published|unpublished|any). true/false are accepted as aliases; empty means
// no filter.
func normalizePublishedStatus(v string) (string, error) {
	switch strings.ToLower(strings.TrimSpace(v)) {
	case "":
		return "", nil
	case "true", "published":
		return "published", nil
	case "false", "unpublished":
		return "unpublished", nil
	case "any":
		return "any", nil
	default:
		return "", output.ErrValidation("--published must be one of published|unpublished|any (or true/false), got %q", v)
	}
}
