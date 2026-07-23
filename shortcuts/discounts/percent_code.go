package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var percentCodeShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+percent-code",
	Use:     "+percent-code --target order|product --percent <1-99> [--limit-max N] [--limit-user N] [--products <ids> | --variants <ids> | --collections <ids>] [--exclude] [--min-amount <amount>] [--min-quantity <n>] [--code <CODE>] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short:   "Create a percent-off discount code (order or product scope)",
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
