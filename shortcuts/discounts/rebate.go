package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var rebateShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+rebate",
	Use: `+rebate --target order|product --tiers "<threshold:discount,...>" [--limit-max N] [--limit-user N] ` +
		`[--type ...] (--products <ids> | --collections <ids> | --variants <ids> — required when --target=product) [--exclude] ` +
		`[--limit-order-once=true|false] [--combines order,product,shipping] [--customer-segments <ids>]`,
	Short: "Create an amount/quantity rebate (order or product scope)",
	Flags: []common.Flag{
		{Name: "target", Type: common.FlagString, Required: true,
			Description: "Rebate scope: order or product (required).",
			Completions: []string{"order", "product"}},
		{Name: "tiers", Type: common.FlagString, Required: true, Description: `Comma-separated "<threshold>:<discount>" pairs, e.g. "100:10,200:25" (spend 100 → save 10; spend 200 → save 25). Threshold = order amount or quantity, discount = reduction amount or percent; units are decided by --type. Required.`},
		{Name: "type", Type: common.FlagString, Default: "amount-off",
			Description: "Rebate type: amount-off | amount-percent | qty-off | qty-percent.",
			Completions: []string{"amount-off", "amount-percent", "qty-off", "qty-percent"}},

		// Scope qualifiers — at most one (mutually exclusive). For --target=order
		// omitting all three means the rebate applies to the whole order
		// (entitled_product.selection=all). For --target=product the API
		// rejects selection=all, so one of these flags is required.
		{Name: "products", Type: common.FlagString, Description: "Product IDs comma-separated (mutex with --collections / --variants; required when --target=product)."},
		{Name: "collections", Type: common.FlagString, Description: "Collection IDs comma-separated (mutex with --products / --variants; required when --target=product)."},
		{Name: "variants", Type: common.FlagString, Description: "Variant IDs comma-separated (mutex with --products / --collections; required when --target=product)."},
		excludeFlag("exclude", ""),

		{Name: "limit-max", Type: common.FlagInt, Description: "Max total discount uses across all customers (>0; omit for no limit)."},
		{Name: "limit-user", Type: common.FlagInt, Description: "Max discount uses per customer (>0; omit for no limit)."},
		{Name: "limit-order-once", Type: common.FlagBool, Default: true, Description: "Apply the discount at most once per order (default true): items beyond the qualifying threshold are charged at regular price. Set false to let the discount repeat across thresholds — e.g. 3 items get $50 off, 6 items get $100 off. Only valid with --type=amount-off|qty-off."},

		{Name: "combines", Type: common.FlagStringSlice, Description: "Allowed combinations (comma-separated subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},
		customerSegmentsFlag(),

		{Name: "name", Type: common.FlagString, Description: "Activity name (auto-generated if omitted)."},
		common.StartTimeFlag(),
		common.EndTimeFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		target := in.Flags.GetString("target")
		if target != "order" && target != "product" {
			return common.PlannedRequest{}, output.ErrValidation("--target must be 'order' or 'product' (got %q)", target)
		}
		layers, err := common.ParseTiers(in.Flags.GetString("tiers"))
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "--tiers: %s", err)
		}
		rt, err := ParseRebateType(in.Flags.GetString("type"))
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "--type: %s", err)
		}
		if err := validateLayerObtainValues(layers, rt.ObtainType == "percent"); err != nil {
			return common.PlannedRequest{}, err
		}

		// --limit-order-once only applies to off-types (obtain_type=fixed_price_reduction);
		// rejecting up-front avoids the API rejecting silently or applying inconsistently.
		if in.Flags.Changed("limit-order-once") && rt.ObtainType == "percent" {
			return common.PlannedRequest{}, output.ErrValidation("--limit-order-once is only valid for --type=amount-off or qty-off (got --type=%s)", in.Flags.GetString("type"))
		}

		productIDs := common.ParseProducts(in.Flags.GetString("products"))
		collectionIDs := common.ParseProducts(in.Flags.GetString("collections"))
		variantIDs := common.ParseProducts(in.Flags.GetString("variants"))
		// --exclude is product-level only; order-level rebate has no exclude option.
		exclude := in.Flags.GetBool("exclude")
		if exclude && target == "order" {
			return common.PlannedRequest{}, output.ErrValidation("--exclude only applies to --target=product (order-level rebate has no exclude option)")
		}
		// Order-level rebate covers the whole order; a product scope is rejected by the API.
		if target == "order" && (len(productIDs) > 0 || len(collectionIDs) > 0 || len(variantIDs) > 0) {
			return common.PlannedRequest{}, output.ErrValidation("--target=order applies to the whole order; remove %s (or use --target=product to limit the rebate to specific items)", defaultScopeNames().scopeList())
		}
		// --target=product needs an explicit product set: the backend rejects
		// entitled_product.selection=all for a product-level rebate (returns 422),
		// so "all products" must be expressed via --target=order. This target-specific
		// message is surfaced here; resolveScope's generic check is the backstop.
		if target == "product" && !exclude && len(productIDs) == 0 && len(collectionIDs) == 0 && len(variantIDs) == 0 {
			return common.PlannedRequest{}, output.ErrValidation("--target=product requires one of %s", defaultScopeNames().scopeList())
		}
		// --target=order may omit scope (selection=all). --exclude flips a
		// supplied scope to selection=exclude.
		entitledProduct, err := resolveScope(productIDs, collectionIDs, variantIDs, exclude, target == "product", defaultScopeNames())
		if err != nil {
			return common.PlannedRequest{}, err
		}

		combines, err := validateCombines(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}

		info, err := buildAutoInfoFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "%s", err)
		}
		info["discount_type"] = rt.DiscountType
		info["discount_target"] = target

		limitMax, limitUser, err := resolveLimitMaxUser(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		rule := map[string]any{
			"discount_combines":   combines,
			"limit_max_discount":  limitMax,
			"limit_user_discount": limitUser,
		}
		if rt.ObtainType == "fixed_price_reduction" {
			limitOrder := 1
			if !in.Flags.GetBool("limit-order-once") {
				limitOrder = -1
			}
			rule["limit_order_discount"] = limitOrder
		}

		payload := map[string]any{
			"discount": map[string]any{
				"discount_info": info,
				"discount_layer": map[string]any{
					"condition_type": rt.ConditionType,
					"obtain_type":    rt.ObtainType,
					"layers":         common.LayersToMaps(layers),
				},
				"discount_rule":     rule,
				"entitled_customer": resolveEntitledCustomer(in),
				"entitled_product":  entitledProduct,
			},
		}
		return PlanCreateAutomatic(payload), nil
	},
}
