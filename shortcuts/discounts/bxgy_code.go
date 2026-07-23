package discounts

import (
	"strconv"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var bxgyCodeShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+bxgy-code",
	Use: "+bxgy-code (--products <ids> | --variants <ids> | --collections <ids>) [--exclude] " +
		"(--buy-quantity N | --buy-amount X) " +
		"(--get-products <ids> | --get-variants <ids> | --get-collections <ids>) --get-quantity N " +
		"[--limit-max N] [--limit-user N] " +
		"(--get-percent <1-99> | --get-off <amount> | --get-free) [--limit-order N] [--code <CODE>] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short: "Create a buy-X-get-Y discount code",
	Flags: []common.Flag{
		// Buy-side scope — mutex; exactly one required.
		{Name: "products", Type: common.FlagString, Description: "Buy-side product IDs comma-separated (mutex with --variants / --collections)."},
		{Name: "variants", Type: common.FlagString, Description: "Buy-side variant IDs comma-separated (mutex with --products / --collections)."},
		{Name: "collections", Type: common.FlagString, Description: "Buy-side collection IDs comma-separated (mutex with --products / --variants)."},
		excludeFlag("exclude", "buy-side "),
		// Buy-side trigger — exactly one required (mutex).
		{Name: "buy-quantity", Type: common.FlagInt, Description: "Total qty across the buy-side items required to trigger the discount (>0; mutex with --buy-amount)."},
		{Name: "buy-amount", Type: common.FlagFloat, Description: "Total spend across the buy-side items required to trigger the discount (>0; mutex with --buy-quantity)."},

		// Get-side scope — mutex; exactly one required.
		{Name: "get-products", Type: common.FlagString, Description: "Get-side product IDs comma-separated (mutex with --get-variants / --get-collections)."},
		{Name: "get-variants", Type: common.FlagString, Description: "Get-side variant IDs comma-separated (mutex with --get-products / --get-collections)."},
		{Name: "get-collections", Type: common.FlagString, Description: "Get-side collection IDs comma-separated (mutex with --get-products / --get-variants)."},
		{Name: "get-quantity", Type: common.FlagInt, Required: true, Description: "How many get-side items receive the discount (required, >0)."},

		// Discount applied to the get items — exactly one required (mutex).
		{Name: "get-percent", Type: common.FlagInt, Description: "Percent off on get items (1-99; mutex with --get-off / --get-free)."},
		{Name: "get-off", Type: common.FlagFloat, Description: "Fixed amount off on get items (>0; mutex with --get-percent / --get-free)."},
		{Name: "get-free", Type: common.FlagBool, Description: "Gift the get-side items for free (mutex with --get-percent / --get-off)."},

		{Name: "code", Type: common.FlagString, Description: "Discount code (auto-generated if omitted)."},
		{Name: "name", Type: common.FlagString, Description: "Activity name (auto-generated if omitted)."},
		common.StartTimeFlag(),
		common.EndTimeFlag(),
		{Name: "combines", Type: common.FlagStringSlice,
			Description: "Allowed combinations (subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},
		{Name: "limit-max", Type: common.FlagInt, Description: "Max total discount uses across all customers (>0; omit for no limit)."},
		{Name: "limit-user", Type: common.FlagInt, Description: "Max discount uses per customer (>0; omit for no limit)."},
		{Name: "limit-order", Type: common.FlagInt, Default: 1, Description: "Max times this discount applies per order (default 1 = single bxgy set per order; pass -1 for no limit)."},
		customerSegmentsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		// Buy-side scope (required); --exclude flips it to selection=exclude
		// ("buy any product EXCEPT these to trigger").
		entitledProduct, err := resolveScope(
			common.ParseProducts(in.Flags.GetString("products")),
			common.ParseProducts(in.Flags.GetString("collections")),
			common.ParseProducts(in.Flags.GetString("variants")),
			in.Flags.GetBool("exclude"), true, defaultScopeNames())
		if err != nil {
			return common.PlannedRequest{}, err
		}
		// Get-side scope (required). The get side has no exclude option, so exclude=false.
		obtainProduct, err := resolveScope(
			common.ParseProducts(in.Flags.GetString("get-products")),
			common.ParseProducts(in.Flags.GetString("get-collections")),
			common.ParseProducts(in.Flags.GetString("get-variants")),
			false, true,
			scopeNames{products: "get-products", collections: "get-collections", variants: "get-variants"})
		if err != nil {
			return common.PlannedRequest{}, err
		}

		// Buy-side trigger: exactly one of --buy-quantity / --buy-amount.
		buyQtySet := in.Flags.Changed("buy-quantity")
		buyAmtSet := in.Flags.Changed("buy-amount")
		if buyQtySet && buyAmtSet {
			return common.PlannedRequest{}, output.ErrValidation("--buy-quantity and --buy-amount are mutually exclusive; set exactly one")
		}
		if !buyQtySet && !buyAmtSet {
			return common.PlannedRequest{}, output.ErrValidation("one of --buy-quantity or --buy-amount is required")
		}
		var condType, condValue string
		if buyQtySet {
			n := in.Flags.GetInt("buy-quantity")
			if n <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--buy-quantity must be > 0 (got %d)", n)
			}
			condType = "purchase_quantity"
			condValue = strconv.Itoa(n)
		} else {
			amt := in.Flags.GetFloat("buy-amount")
			if amt <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--buy-amount must be > 0 (got %v)", amt)
			}
			condType = "purchase_amount"
			condValue = strconv.FormatFloat(amt, 'f', -1, 64)
		}

		getQty := in.Flags.GetInt("get-quantity")
		if getQty <= 0 {
			return common.PlannedRequest{}, output.ErrValidation("--get-quantity must be > 0 (got %d)", getQty)
		}

		// Discount on get items: exactly one of --get-percent / --get-off / --get-free.
		gotPercent := in.Flags.Changed("get-percent")
		gotOff := in.Flags.Changed("get-off")
		gotFree := in.Flags.Changed("get-free")
		setCount := 0
		for _, b := range []bool{gotPercent, gotOff, gotFree} {
			if b {
				setCount++
			}
		}
		if setCount == 0 {
			return common.PlannedRequest{}, output.ErrValidation("one of --get-percent / --get-off / --get-free is required")
		}
		if setCount > 1 {
			return common.PlannedRequest{}, output.ErrValidation("--get-percent / --get-off / --get-free are mutually exclusive; set exactly one")
		}
		obtainType := "free_acquisition"
		obtainValue := "0"
		switch {
		case gotPercent:
			pct := in.Flags.GetInt("get-percent")
			if pct < 1 || pct > 99 {
				return common.PlannedRequest{}, output.ErrValidation("--get-percent must be between 1 and 99 (got %d)", pct)
			}
			obtainType = "percent"
			obtainValue = strconv.Itoa(pct)
		case gotOff:
			amt := in.Flags.GetFloat("get-off")
			if amt <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--get-off must be > 0 (got %v)", amt)
			}
			obtainType = "fixed_price_reduction"
			obtainValue = strconv.FormatFloat(amt, 'f', -1, 64)
		}

		info, err := buildCodeInfoFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "%s", err)
		}
		info["discount_type"] = "code_bxgy"
		info["discount_target"] = "product"

		rule, err := codeRuleFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		// Server-side validation requires limit_order_discount for bxgy.
		// Defaults to 1 (single bxgy set per order — safe against stacking);
		// --limit-order -1 opts into unlimited.
		rule["limit_order_discount"] = in.Flags.GetInt("limit-order")

		payload := map[string]any{
			"discount": map[string]any{
				"discount_info": info,
				"discount_layer": map[string]any{
					"condition_type": condType,
					"obtain_type":    obtainType,
					"layers": []any{map[string]any{
						"condition_value": condValue,
						"obtain_value":    obtainValue,
						"obtain_count":    getQty,
					}},
				},
				"discount_rule":     rule,
				"entitled_customer": resolveEntitledCustomer(in),
				"entitled_product":  entitledProduct,
				"obtain_product":    obtainProduct,
			},
		}
		return PlanCreateNonAutomatic(payload), nil
	},
}
