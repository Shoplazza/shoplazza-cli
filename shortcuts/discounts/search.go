package discounts

import (
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// ── +search ───────────────────────────────────────────────────────────────────

var discountSources = []struct{ name, id string }{
	{"manual", "1"}, {"goaffpro", "2"}, {"fintouch", "3"}, {"mambasms", "4"},
	{"stamped", "5"}, {"recommendation", "6"}, {"athena", "10"}, {"other", "999"},
}

var searchShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+search",
	Use:     "+search",
	Short:   "Search discounts by keyword or code",
	Long:    "Search discounts by name, code, type, target, progress, start time, stacking, source, or ID. " + common.OnePage,
	Example: `  # Find a discount by code
  shoplazza discounts +search --discount-code SAVE20

  # Ongoing percent-off code discounts, 50 per page
  shoplazza discounts +search --progress ongoing --discount-type code_percent --page-limit 50

  # Discounts made by hand that start in December 2026 (UTC+8)
  shoplazza discounts +search --source manual --since 2026-12-01T00:00:00+08:00 --until 2026-12-31T23:59:59+08:00`,
	Flags: []common.Flag{
		{Name: "query", Type: common.FlagString, Description: "Filter by discount name: matches names that contain the whole value as one case-insensitive substring (no word splitting or fuzzy matching), so \"Spring Sale 2026\" does not find \"Spring Sale\"."},
		{Name: "discount-code", Type: common.FlagString, Description: "Filter by discount code."},
		{Name: "progress", Type: common.FlagStringSlice, Description: "Filter by progress: ongoing|not_started|finished|paused.",
			Enum: []string{"ongoing", "not_started", "finished", "paused"}},
		{Name: "discount-type", Type: common.FlagStringSlice, Description: "Filter by discount type: flashsale|rebate_cta_otr|rebate_ctq_otr|rebate_cta_otp|rebate_ctq_otp|m_n_discount|code_percent|code_fix_price_reduction|code_bxgy|code_free_shipping.",
			Enum: []string{
				"flashsale",
				"rebate_cta_otr", "rebate_ctq_otr", "rebate_cta_otp", "rebate_ctq_otp",
				"m_n_discount",
				"code_percent", "code_fix_price_reduction", "code_bxgy", "code_free_shipping",
			}},
		{Name: "discount-target", Type: common.FlagStringSlice, Description: "Filter by target: product|order|shipping.",
			Enum: []string{"order", "product", "shipping"}},
		{Name: "discount-method", Type: common.FlagStringSlice, Description: "Filter by method: automatic|discount_code.",
			Enum: []string{"automatic", "discount_code"}},
		{Name: "since", Type: common.FlagString, Description: "Only discounts that start at or after this time (starts_date); one that started earlier is left out even if it is still running. Accepts " + common.TimeFormats + "."},
		{Name: "until", Type: common.FlagString, Description: "Only discounts that start at or before this time (ends_date). Accepts " + common.TimeFormats + "."},
		{Name: "combines", Type: common.FlagStringSlice, Description: "Only discounts that stack with any of: order|product|shipping.",
			Enum: []string{"order", "product", "shipping"}},
		{Name: "source", Type: common.FlagStringSlice, Description: "Who created the discount: manual|goaffpro|fintouch|mambasms|stamped|recommendation|athena|other, or a numeric source ID. Discounts made through the API, this CLI included, are other."},
		{Name: "ids", Type: common.FlagStringSlice, Description: "Discount IDs, comma-separated."},
		common.PageLimitFlag(),
		common.CursorFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q, err := searchQuery(in.Flags)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		if err := common.AddTimeRangeAs(q, in, "starts_date", "ends_date"); err != nil {
			return common.PlannedRequest{}, err
		}
		openSearchWindow(q)
		return PlanList(q), nil
	},
}

func searchQuery(f common.FlagSet) (map[string]any, error) {
	q := map[string]any{}
	cmdutil.AddString(q, "discount_name", f.GetString("query"))
	cmdutil.AddString(q, "discount_code", f.GetString("discount-code"))
	cmdutil.AddSlice(q, "progress", f.GetStringSlice("progress"))
	cmdutil.AddSlice(q, "discount_type", f.GetStringSlice("discount-type"))
	cmdutil.AddSlice(q, "discount_targets", f.GetStringSlice("discount-target"))
	cmdutil.AddSlice(q, "discount_methods", f.GetStringSlice("discount-method"))
	cmdutil.AddSlice(q, "discount_combines", f.GetStringSlice("combines"))
	cmdutil.AddSlice(q, "ids", f.GetStringSlice("ids"))
	if f.Changed("source") {
		ids, err := sourceIDs(f.GetStringSlice("source"))
		if err != nil {
			return nil, err
		}
		q["source_ids"] = ids
	}
	if ps := f.GetInt("page-limit"); ps > 0 {
		q["page_size"] = ps
	}
	cmdutil.AddString(q, "cursor", f.GetString("cursor"))
	return q, nil
}

func sourceIDs(values []string) ([]string, error) {
	ids := make([]string, 0, len(values))
next:
	for _, v := range values {
		v = strings.TrimSpace(v)
		if _, err := strconv.ParseUint(v, 10, 64); err == nil {
			ids = append(ids, v)
			continue
		}
		names := make([]string, 0, len(discountSources))
		for _, s := range discountSources {
			if strings.EqualFold(v, s.name) {
				ids = append(ids, s.id)
				continue next
			}
			names = append(names, s.name)
		}
		return nil, output.ErrValidation("--source %q is neither a source name nor a numeric source ID", v).
			WithParam("--source").
			WithSubtype(output.SubtypeInvalidFlagValue).
			WithHint("source names: " + strings.Join(names, ", "))
	}
	return ids, nil
}

// openSearchWindow sends -1 (no bound) for the missing side: the server reads a lone starts_date or ends_date as a lower bound.
func openSearchWindow(q map[string]any) {
	_, hasStart := q["starts_date"]
	_, hasEnd := q["ends_date"]
	switch {
	case hasStart && !hasEnd:
		q["ends_date"] = "-1"
	case hasEnd && !hasStart:
		q["starts_date"] = "-1"
	}
}
