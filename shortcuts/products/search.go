package products

import (
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var searchShortcut = common.Shortcut{
	Service: "products",
	Command: "+search",
	Use:     "+search",
	Short:   "Quickly search products",
	Long:    "Search the catalog by title, vendor, collection, or published status. Returns one page — set --page-limit and follow has_more/cursor for the rest.",
	Example: `  # Published products by title, 50 per page
  shoplazza products +search --keyword shirt --published published --page-limit 50

  # By vendor, print only titles
  shoplazza products +search --vendor Acme --jq '.data.products[].title'

  # Only a few fields per product
  shoplazza products +search --fields id,title,primary_image,price_min`,
	Flags: []common.Flag{
		{Name: "keyword", Type: common.FlagString, Description: "Filter by product title."},
		{Name: "published", Type: common.FlagString, Description: "Filter by published status: published, unpublished, any (true/false also accepted).", Completions: []string{"published", "unpublished", "any"}},
		{Name: "vendor", Type: common.FlagString, Description: "Filter by vendor name (exact match)."},
		{Name: "collection-id", Type: common.FlagString, Description: "Filter by collection ID."},
		common.PageLimitFlag(),
		{Name: "fields", Type: common.FlagStringSlice, Description: "Product fields to return, by response key (comma-separated, e.g. id,title,primary_image). created_at/updated_at may also come back; unknown names are silently ignored."},
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
		if fields := listFieldSelectors(in.Flags.GetStringSlice("fields")); len(fields) > 0 {
			q["fields"] = fields
		}
		return PlanList(q), nil
	},
}

// fieldSelectors maps response keys to the list API's differently named `fields` selectors.
var fieldSelectors = map[string]string{
	"primary_image":    "image",
	"origin_price_min": "price_min",
	"origin_price_max": "price_max",
}

// listFieldSelectors translates --fields response keys to API selectors, trimmed and deduped.
func listFieldSelectors(fields []string) []string {
	var out []string
	seen := map[string]bool{}
	for _, f := range fields {
		f = strings.TrimSpace(f)
		if s, ok := fieldSelectors[f]; ok {
			f = s
		}
		if f == "" || seen[f] {
			continue
		}
		seen[f] = true
		out = append(out, f)
	}
	return out
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
