package discounts

import (
	"strconv"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

var flashsaleShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+flashsale",
	Use: "+flashsale --value <n> " +
		"[--type percent|fixed-price|off] " +
		"[--variants <ids> | --collections <ids>] " +
		"[--price-rule price|compare_at_price] " +
		"[--limit-user-variant N | --limit-user-product N | --limit-user-all N] " +
		"[--stock N] [--combines order,product,shipping] [--customer-segments <ids>]",
	Short: "Create a product flash sale",
	Flags: []common.Flag{
		{Name: "value", Type: common.FlagFloat, Required: true,
			Description: "Discount value; meaning depends on --type: percent → 1-99 (% off); fixed-price → the new selling price; off → amount to subtract. Required."},
		{Name: "type", Type: common.FlagString, Default: "percent",
			Description: "Discount type: percent | fixed-price | off.",
			Completions: []string{"percent", "fixed-price", "off"}},

		// Scope qualifiers — at most one (mutually exclusive). Flashsale operates
		// on SKUs, not parent products, so there is no --products flag.
		{Name: "variants", Type: common.FlagString, Description: "Variant IDs comma-separated (mutex with --collections)."},
		{Name: "collections", Type: common.FlagString, Description: "Collection IDs comma-separated (mutex with --variants)."},

		{Name: "price-rule", Type: common.FlagString, Default: "price",
			Description: "Which price the discount applies to: price (selling price) or compare_at_price (list price).",
			Completions: []string{"price", "compare_at_price"}},
		{Name: "limit-user-variant", Type: common.FlagInt, Description: "Max items per user per variant (>0; mutex with --limit-user-product / --limit-user-all; omit all three → no per-user limit)."},
		{Name: "limit-user-product", Type: common.FlagInt, Description: "Max items per user per product (>0; mutex with --limit-user-variant / --limit-user-all; omit all three → no per-user limit)."},
		{Name: "limit-user-all", Type: common.FlagInt, Description: "Max items per user across all activity products (>0; mutex with --limit-user-variant / --limit-user-product; omit all three → no per-user limit)."},
		{Name: "stock", Type: common.FlagInt, Description: "Discount-specific stock cap (>0; omit to follow product stock)."},
		{Name: "combines", Type: common.FlagStringSlice,
			Description: "Allowed combinations (subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},

		{Name: "name", Type: common.FlagString, Description: "Activity name (auto-generated if omitted)."},
		common.StartTimeFlag(),
		common.EndTimeFlag(),
		customerSegmentsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		obtainType, err := ParseFlashsaleType(in.Flags.GetString("type"))
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "--type: %s", err)
		}

		value := in.Flags.GetFloat("value")
		if obtainType == "percent" {
			if value < 1 || value > 99 {
				return common.PlannedRequest{}, output.ErrValidation("--value must be 1-99 when --type=percent (got %v)", value)
			}
		} else if value <= 0 {
			return common.PlannedRequest{}, output.ErrValidation("--value must be > 0 (got %v)", value)
		}

		variantIDs := common.ParseProducts(in.Flags.GetString("variants"))
		collectionIDs := common.ParseProducts(in.Flags.GetString("collections"))
		if len(variantIDs) > 0 && len(collectionIDs) > 0 {
			return common.PlannedRequest{}, output.ErrValidation("--variants and --collections are mutually exclusive; set at most one")
		}

		priceRule := in.Flags.GetString("price-rule")
		switch priceRule {
		case "price", "compare_at_price":
		default:
			return common.PlannedRequest{}, output.ErrValidation("--price-rule %q invalid; allowed: price, compare_at_price", priceRule)
		}

		limitUserType := "no_limit"
		limitUserCount := -1
		var chosenLimit string
		for _, name := range []string{"limit-user-variant", "limit-user-product", "limit-user-all"} {
			if !in.Flags.Changed(name) {
				continue
			}
			if chosenLimit != "" {
				return common.PlannedRequest{}, output.ErrValidation("--limit-user-variant / --limit-user-product / --limit-user-all are mutually exclusive; set at most one")
			}
			chosenLimit = name
		}
		if chosenLimit != "" {
			limitUserCount = in.Flags.GetInt(chosenLimit)
			if limitUserCount <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--%s must be > 0 (got %d)", chosenLimit, limitUserCount)
			}
			switch chosenLimit {
			case "limit-user-variant":
				limitUserType = "customer_variant"
			case "limit-user-product":
				limitUserType = "customer_product"
			case "limit-user-all":
				limitUserType = "customer_all_product"
			}
		}

		// --stock presence drives inventory source: unset → follow product stock;
		// set → use the value as a discount-specific cap.
		followStock := "product"
		stock := 0
		if in.Flags.Changed("stock") {
			stock = in.Flags.GetInt("stock")
			if stock <= 0 {
				return common.PlannedRequest{}, output.ErrValidation("--stock must be > 0 (got %d)", stock)
			}
			followStock = "discount"
		}

		combines, err := validateCombines(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}

		info, err := buildAutoInfoFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "%s", err)
		}
		info["discount_type"] = "flashsale"
		info["discount_target"] = "product"

		// Flashsale uses variant_ids (SKUs), never product_ids, so pass nil for
		// that slot. Scope is optional (empty = all) and there is no exclude option.
		entitledProduct, err := resolveScope(nil, collectionIDs, variantIDs, false, false,
			scopeNames{collections: "collections", variants: "variants"})
		if err != nil {
			return common.PlannedRequest{}, err
		}

		payload := map[string]any{
			"discount": map[string]any{
				"discount_info": info,
				// discount_layer holds only condition_type / obtain_type / layers;
				// rule-level fields (follow_stock / price_rule / limit_user_product_type)
				// live in discount_rule.
				"discount_layer": map[string]any{
					"condition_type": "no_condition",
					"obtain_type":    obtainType,
					"layers": []any{map[string]any{
						"obtain_value": strconv.FormatFloat(value, 'f', -1, 64),
					}},
				},
				"discount_rule": map[string]any{
					"discount_combines":           combines,
					"price_rule":                  priceRule,
					"limit_user_product_type":     limitUserType,
					"limit_user_product_discount": limitUserCount,
					"follow_stock":                followStock,
					"stock":                       stock,
					// flashsale exposes no --limit-max / --limit-user, but the API
					// requires both fields; send -1 = no limit on both.
					"limit_max_discount":  -1,
					"limit_user_discount": -1,
				},
				"entitled_customer": resolveEntitledCustomer(in),
				"entitled_product":  entitledProduct,
			},
		}
		return PlanCreateAutomatic(payload), nil
	},
}
