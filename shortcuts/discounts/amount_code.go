package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var amountCodeShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+amount-code",
	Use:     "+amount-code --target order|product --off <amount> [--limit-max N] [--limit-user N] [--products <ids> | --variants <ids> | --collections <ids>] [--exclude] [--min-amount <amount>] [--min-quantity <n>] [--limit-order-once=true|false] [--code <CODE>] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short:   "Create a fixed-amount-off discount code (order or product scope)",
	Long:    "Create a fixed-amount-off discount code scoped to the whole order or specific products; run --dry-run first to preview the request.",
	Example: `  # Preview a $10-off order code
  shoplazza discounts +amount-code --target order --off 10 --code TENOFF --dry-run

  # $5 off specific products with a minimum order amount
  shoplazza discounts +amount-code --target product --off 5 --products p-1,p-2 --min-amount 50`,
	Flags: append(codeOffFlags(),
		common.Flag{
			Name:        "off",
			Type:        common.FlagFloat,
			Required:    true,
			Description: "Fixed amount off (required).",
		},
		common.Flag{
			Name:        "limit-order-once",
			Type:        common.FlagBool,
			Default:     true,
			Description: "Apply the discount at most once per order (default true): items beyond the qualifying threshold are charged at regular price. Set false to let the discount repeat across thresholds — e.g. 3 items get $50 off, 6 items get $100 off.",
		},
	),
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		off := in.Flags.GetFloat("off")
		if off <= 0 {
			return common.PlannedRequest{}, output.ErrValidation("--off must be > 0 (got %v)", off)
		}
		payload, err := buildCodeDiscountPayload(in, "code_fix_price_reduction", "fixed_price_reduction", off)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		// Server-side validation requires limit_order_discount for
		// fixed_price_reduction. Default 1 = one use per order;
		// --limit-order-once=false opts into unlimited (-1).
		limitOrder := 1
		if !in.Flags.GetBool("limit-order-once") {
			limitOrder = -1
		}
		payload["discount"].(map[string]any)["discount_rule"].(map[string]any)["limit_order_discount"] = limitOrder
		return PlanCreateNonAutomatic(payload), nil
	},
}
