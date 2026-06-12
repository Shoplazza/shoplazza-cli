package discounts

import (
	"strconv"

	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

// freeShippingFullCoverageObtainValue expresses "100% free shipping" as a
// fixed-amount reduction larger than any realistic shipping fee, because
// code_free_shipping requires obtain_type=fixed_price_reduction (percent is rejected).
const freeShippingFullCoverageObtainValue = "999999999"

var freeShippingCodeShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+free-shipping-code",
	Use:     "+free-shipping-code [--limit-max N] [--limit-user N] [--off <amount>] [--min-amount <amount> | --min-quantity <n>] [--countries <ISO,...|all>] [--code <CODE>] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short:   "Create a free-shipping discount code",
	Flags: []common.Flag{
		{Name: "off", Type: common.FlagFloat, Description: "Fixed amount off shipping (omit = 100% free shipping)."},
		{Name: "min-amount", Type: common.FlagFloat, Description: "Minimum order amount (mutex with --min-quantity)."},
		{Name: "min-quantity", Type: common.FlagInt, Description: "Minimum item count (mutex with --min-amount)."},
		{Name: "countries", Type: common.FlagString, Description: "ISO country codes (comma-separated) or 'all'."},
		{Name: "code", Type: common.FlagString, Description: "Discount code (auto-generated if omitted)."},
		{Name: "name", Type: common.FlagString, Description: "Activity name (auto-generated if omitted)."},
		common.StartTimeFlag(),
		common.EndTimeFlag(),
		{Name: "combines", Type: common.FlagStringSlice,
			Description: "Allowed combinations (subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},
		{Name: "limit-max", Type: common.FlagInt, Description: "Max total discount uses across all customers (>0; omit for no limit)."},
		{Name: "limit-user", Type: common.FlagInt, Description: "Max discount uses per customer (>0; omit for no limit)."},
		customerSegmentsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		info, err := buildCodeInfoFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "%s", err)
		}
		info["discount_type"] = "code_free_shipping"
		// Server-side validation requires discount_target. Hard-code "shipping"
		// (the only valid target for this discount type).
		info["discount_target"] = "shipping"

		// Mutex: at most one of --min-amount / --min-quantity.
		minAmountSet := in.Flags.Changed("min-amount")
		minQtySet := in.Flags.Changed("min-quantity")
		if minAmountSet && minQtySet {
			return common.PlannedRequest{}, output.ErrValidation("--min-amount and --min-quantity are mutually exclusive; set at most one")
		}

		condType := "no_condition"
		condValue := "0"
		switch {
		case minAmountSet:
			amt := in.Flags.GetFloat("min-amount")
			if amt <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--min-amount must be > 0 (got %v)", amt)
			}
			condType = "purchase_amount"
			condValue = strconv.FormatFloat(amt, 'f', -1, 64)
		case minQtySet:
			qty := in.Flags.GetInt("min-quantity")
			if qty <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--min-quantity must be > 0 (got %d)", qty)
			}
			condType = "purchase_quantity"
			condValue = strconv.Itoa(qty)
		}

		entitledArea := map[string]any{}
		countries := in.Flags.GetString("countries")
		if countries != "" && countries != "all" {
			codes := common.ParseProducts(countries)
			if len(codes) > 0 {
				areaObjs := make([]any, len(codes))
				for i, c := range codes {
					areaObjs[i] = map[string]any{"country_code": c}
				}
				entitledArea["areas"] = areaObjs
			}
		}

		rule, err := codeRuleFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}

		// See freeShippingFullCoverageObtainValue's doc comment for why we use
		// fixed_price_reduction with a sentinel-large value instead of percent.
		obtainType := "fixed_price_reduction"
		obtainValue := freeShippingFullCoverageObtainValue
		if in.Flags.Changed("off") {
			off := in.Flags.GetFloat("off")
			if off <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--off must be > 0 (got %v)", off)
			}
			obtainValue = strconv.FormatFloat(off, 'f', -1, 64)
		}

		// Server-side validation requires layer.obtain_value (length >= 1).
		payload := map[string]any{
			"discount": map[string]any{
				"discount_info": info,
				"discount_layer": map[string]any{
					"condition_type": condType,
					"obtain_type":    obtainType,
					"layers": []any{map[string]any{
						"condition_value": condValue,
						"obtain_value":    obtainValue,
					}},
				},
				"discount_rule":     rule,
				"entitled_customer": resolveEntitledCustomer(in),
				"entitled_area":     entitledArea,
			},
		}
		return PlanCreateNonAutomatic(payload), nil
	},
}
