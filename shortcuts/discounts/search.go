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
		{Name: "source", Type: common.FlagStringSlice, Description: "Who created the discount: " + strings.Join(sourceNames(), "|") + ", or a numeric source ID. Discounts made through the API, this CLI included, are other.",
			Completions: sourceNames()},
		{Name: "ids", Type: common.FlagStringSlice, Description: "Discount IDs, comma-separated."},
		common.PageLimitFlag(),
		common.CursorFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "discount_name", in.Flags.GetString("query"))
		cmdutil.AddString(q, "discount_code", in.Flags.GetString("discount-code"))
		cmdutil.AddSlice(q, "progress", in.Flags.GetStringSlice("progress"))
		cmdutil.AddSlice(q, "discount_type", in.Flags.GetStringSlice("discount-type"))
		cmdutil.AddSlice(q, "discount_targets", in.Flags.GetStringSlice("discount-target"))
		cmdutil.AddSlice(q, "discount_methods", in.Flags.GetStringSlice("discount-method"))
		cmdutil.AddSlice(q, "discount_combines", in.Flags.GetStringSlice("combines"))
		cmdutil.AddSlice(q, "ids", in.Flags.GetStringSlice("ids"))
		if in.Flags.Changed("source") {
			ids, err := sourceIDs(in.Flags.GetStringSlice("source"))
			if err != nil {
				return common.PlannedRequest{}, err
			}
			q["source_ids"] = ids
		}
		if ps := in.Flags.GetInt("page-limit"); ps > 0 {
			q["page_size"] = ps
		}
		cmdutil.AddString(q, "cursor", in.Flags.GetString("cursor"))
		// -1 leaves a side unbounded: the server reads a lone starts_date or ends_date as a lower bound.
		if in.Flags.Changed("since") || in.Flags.Changed("until") {
			q["starts_date"], q["ends_date"] = "-1", "-1"
		}
		if err := common.AddTimeRangeAs(q, in, "starts_date", "ends_date"); err != nil {
			return common.PlannedRequest{}, err
		}
		return PlanList(q), nil
	},
}

func sourceNames() []string {
	names := make([]string, len(discountSources))
	for i, s := range discountSources {
		names[i] = s.name
	}
	return names
}

func sourceIDs(values []string) ([]string, error) {
	ids := make([]string, 0, len(values))
	for _, v := range values {
		id, ok := sourceID(v)
		if !ok {
			return nil, output.ErrValidation("--source %q is neither a source name nor a numeric source ID", v).
				WithParam("--source").
				WithSubtype(output.SubtypeInvalidFlagValue).
				WithHint("accepted: " + strings.Join(sourceNames(), ", ") + ", or a numeric source ID")
		}
		ids = append(ids, id)
	}
	return ids, nil
}

func sourceID(v string) (string, bool) {
	if _, err := strconv.ParseUint(v, 10, 64); err == nil {
		return v, true
	}
	for _, s := range discountSources {
		if strings.EqualFold(v, s.name) {
			return s.id, true
		}
	}
	return "", false
}
