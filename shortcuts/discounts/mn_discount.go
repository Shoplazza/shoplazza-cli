package discounts

import (
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var mnDiscountShortcut = common.Shortcut{
	Service: "discounts",
	Command: "+mn-discount",
	Use: `+mn-discount --tiers "<nth:discount%,...>" [--limit-max N] [--limit-user N] ` +
		`[--scope highest|all|highest-all] ` +
		`[--products <ids> | --collections <ids> | --variants <ids>] [--exclude] ` +
		`[--price-sort desc|asc] [--combines order,product,shipping] [--customer-segments <ids>]`,
	Short: `Create a quantity-tiered "buy N, Nth-item percent off" discount (e.g. buy 3, 3rd item 50% off)`,
	Flags: []common.Flag{
		{Name: "tiers", Type: common.FlagString, Required: true, Description: `Comma-separated "<nth>:<percent>" pairs, e.g. "2:30,3:50" (buy 2 items → 2nd is 30% off; buy 3 → 3rd is 50% off). Required.`},
		{Name: "scope", Type: common.FlagString, Default: "highest",
			Description: "Which tiers apply: highest | all | highest-all.",
			Completions: []string{"highest", "all", "highest-all"}},

		// Scope qualifiers — at most one. Empty = all-products selection.
		{Name: "products", Type: common.FlagString, Description: "Product IDs comma-separated (mutex with --collections / --variants)."},
		{Name: "collections", Type: common.FlagString, Description: "Collection IDs comma-separated (mutex with --products / --variants)."},
		{Name: "variants", Type: common.FlagString, Description: "Variant IDs comma-separated (mutex with --products / --collections)."},
		excludeFlag("exclude", ""),

		// discount_rule.* fields (previously hardcoded).
		{Name: "price-sort", Type: common.FlagString, Default: "desc",
			Description: `Price-sort order when mapping tier % onto products. desc (default): sort high→low, so higher-priced items get the smallest % off (lower customer savings). asc: sort low→high, so higher-priced items get the largest % off (higher customer savings).`,
			Completions: []string{"desc", "asc"}},
		{Name: "combines", Type: common.FlagStringSlice,
			Description: "Allowed combinations (subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},
		{Name: "limit-max", Type: common.FlagInt, Description: "Max total discount uses across all customers (>0; omit for no limit)."},
		{Name: "limit-user", Type: common.FlagInt, Description: "Max discount uses per customer (>0; omit for no limit)."},

		{Name: "name", Type: common.FlagString, Description: "Activity name (auto-generated if omitted)."},
		common.StartTimeFlag(),
		common.EndTimeFlag(),
		customerSegmentsFlag(),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		layers, err := common.ParseTiers(in.Flags.GetString("tiers"))
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "--tiers: %s", err)
		}
		if err := validateLayerObtainValues(layers, true); err != nil {
			return common.PlannedRequest{}, err
		}

		// --scope flag → mn_discount_scope. UI uses "highest-all" (hyphen);
		// API expects "highest_all" (underscore). Map explicitly.
		var mnScope string
		switch in.Flags.GetString("scope") {
		case "all":
			mnScope = "all"
		case "highest-all":
			mnScope = "highest_all"
		case "highest", "":
			mnScope = "highest"
		default:
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "--scope %q is invalid; valid: highest, all, highest-all", in.Flags.GetString("scope"))
		}

		productIDs := common.ParseProducts(in.Flags.GetString("products"))
		collectionIDs := common.ParseProducts(in.Flags.GetString("collections"))
		variantIDs := common.ParseProducts(in.Flags.GetString("variants"))
		// Scope is optional (empty = all products); --exclude flips a supplied
		// scope to selection=exclude.
		entitledProduct, err := resolveScope(productIDs, collectionIDs, variantIDs, in.Flags.GetBool("exclude"), false, defaultScopeNames())
		if err != nil {
			return common.PlannedRequest{}, err
		}

		priceSort := in.Flags.GetString("price-sort")
		switch priceSort {
		case "desc", "asc":
		default:
			return common.PlannedRequest{}, output.ErrValidation("--price-sort %q invalid; allowed: desc, asc", priceSort)
		}

		combines, err := validateCombines(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}

		info, err := buildAutoInfoFromFlags(in)
		if err != nil {
			return common.PlannedRequest{}, output.Errorf(output.ExitValidation, "validation", "%s", err)
		}
		info["discount_type"] = "m_n_discount"
		info["discount_target"] = "product"

		limitMax, limitUser, err := resolveLimitMaxUser(in)
		if err != nil {
			return common.PlannedRequest{}, err
		}
		payload := map[string]any{
			"discount": map[string]any{
				"discount_info": info,
				"discount_layer": map[string]any{
					"condition_type": "purchase_quantity",
					"obtain_type":    "percent",
					"layers":         common.LayersToMaps(layers),
				},
				"discount_rule": map[string]any{
					"mn_discount_scope":      mnScope,
					"product_discount_order": priceSort,
					"discount_combines":      combines,
					"limit_max_discount":     limitMax,
					"limit_user_discount":    limitUser,
				},
				"entitled_customer": resolveEntitledCustomer(in),
				"entitled_product":  entitledProduct,
			},
		}
		return PlanCreateAutomatic(payload), nil
	},
}
