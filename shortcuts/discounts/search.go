package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// ── +search ───────────────────────────────────────────────────────────────────

var searchShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+search",
	Use:     "+search",
	Short:   "Search discounts by keyword or code",
	Flags: []common.Flag{
		{Name: "query", Type: common.FlagString, Description: "Filter by discount name (fuzzy)."},
		{Name: "discount-code", Type: common.FlagString, Description: "Filter by discount code."},
		{Name: "progress", Type: common.FlagStringSlice, Description: "Filter by progress: ongoing|not_started|finished|paused.",
			Completions: []string{"ongoing", "not_started", "finished", "paused"}},
		{Name: "discount-type", Type: common.FlagStringSlice, Description: "Filter by discount type.",
			Completions: []string{
				"flashsale",
				"rebate_cta_otr", "rebate_ctq_otr", "rebate_cta_otp", "rebate_ctq_otp",
				"m_n_discount",
				"code_percent", "code_fix_price_reduction", "code_bxgy", "code_free_shipping",
			}},
		{Name: "discount-target", Type: common.FlagStringSlice, Description: "Filter by target: product|order|shipping.",
			Completions: []string{"order", "product", "shipping"}},
		{Name: "discount-method", Type: common.FlagStringSlice, Description: "Filter by method: automatic|discount_code.",
			Completions: []string{"automatic", "discount_code"}},
		common.PageLimitFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		q := map[string]any{}
		cmdutil.AddString(q, "discount_name", in.Flags.GetString("query"))
		cmdutil.AddString(q, "discount_code", in.Flags.GetString("discount-code"))
		cmdutil.AddSlice(q, "progress", in.Flags.GetStringSlice("progress"))
		cmdutil.AddSlice(q, "discount_type", in.Flags.GetStringSlice("discount-type"))
		cmdutil.AddSlice(q, "discount_targets", in.Flags.GetStringSlice("discount-target"))
		cmdutil.AddSlice(q, "discount_methods", in.Flags.GetStringSlice("discount-method"))
		if ps := in.Flags.GetInt("page-limit"); ps > 0 {
			q["page_size"] = ps
		}
		return PlanList(q), nil
	},
}
