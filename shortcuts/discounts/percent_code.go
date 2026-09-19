package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var percentCodeShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+percent-code",
	Use:     "+percent-code --target order|product --percent <1-99> [--limit-max N] [--limit-user N] [--products <ids> | --variants <ids> | --collections <ids>] [--exclude] [--min-amount <amount>] [--min-quantity <n>] [--code <CODE>] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short:   "Create a percent-off discount code (order or product scope)",
	Long:    "Create a percent-off discount code scoped to the whole order or specific products; run --dry-run first to preview the request.",
	Example: `  # Preview a 20% order-wide code
  shoplazza discounts +percent-code --target order --percent 20 --code SAVE20 --dry-run

  # 15% off specific products, capped total uses
  shoplazza discounts +percent-code --target product --percent 15 --products p-1,p-2 --limit-max 500`,
	Flags: append(codeOffFlags(),
		common.Flag{
			Name:        "percent",
			Type:        common.FlagFloat,
			Required:    true,
			Description: "Percent off (1-99; required).",
		},
	),
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		percent := in.Flags.GetFloat("percent")
		if percent < 1 || percent > 99 {
			return common.PlannedRequest{}, output.ErrValidation("--percent must be 1-99 (got %v)", percent)
		}
		payload, err := buildCodeDiscountPayload(in, "code_percent", "percent", percent)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		return PlanCreateNonAutomatic(payload), nil
	},
}
