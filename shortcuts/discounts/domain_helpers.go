package discounts

import (
	"fmt"
	"math/rand"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const codeChars = "ABCDEFGHIJKLMNOPQRSTUVWXYZ0123456789"

// customerSegmentsFlag is the standardized --customer-segments flag.
// Used by every discount shortcut to populate entitled_customer.customer_segment_ids.
func customerSegmentsFlag() common.Flag {
	return common.Flag{
		Name:        "customer-segments",
		Type:        common.FlagStringSlice,
		Description: "Customer segment IDs comma-separated; only customers in any of these segments can use the discount (omit = all customers eligible).",
	}
}

// resolveEntitledCustomer builds the entitled_customer payload from --customer-segments.
// Returns {} when unset (no restriction); {customer_segment_ids: [...]} otherwise.
func resolveEntitledCustomer(in common.PlanInput) map[string]any {
	ids := common.IDList(in.Flags, "customer-segments")
	if len(ids) == 0 {
		return map[string]any{}
	}
	return map[string]any{"customer_segment_ids": ids}
}

// scopeNames carries the flag names used when building error messages for one
// product-scope "side". The buy/get sides of +bxgy-code differ (get- prefix),
// and +flashsale leaves products empty because it has no --products flag.
type scopeNames struct {
	products, collections, variants, exclude string
}

// defaultScopeNames is the standard --products / --collections / --variants /
// --exclude set used by every shortcut except the get-side of +bxgy-code.
func defaultScopeNames() scopeNames {
	return scopeNames{"products", "collections", "variants", "exclude"}
}

// scopeList renders the applicable scope flags as "--a / --b / --c", skipping
// any name left empty (e.g. +flashsale exposes no --products).
func (n scopeNames) scopeList() string {
	var parts []string
	for _, s := range []string{n.products, n.collections, n.variants} {
		if s != "" {
			parts = append(parts, "--"+s)
		}
	}
	return strings.Join(parts, " / ")
}

// resolveScope is the single source of truth for the entitled_product /
// obtain_product payload across every discount shortcut. The three id-lists
// are mutually exclusive; the exclude toggle flips the selection strategy:
//
//	one id-list, exclude=false → {selection: entitled, <kind>_ids}
//	one id-list, exclude=true  → {selection: exclude,  <kind>_ids}
//	all empty,   exclude=false → {selection: all}        (requireScope=false only)
//	all empty,   requireScope  → error: a scope is required
//	all empty,   exclude=true  → error: exclude needs a scope
//	more than one id-list      → mutex error
//
// requireScope=true is for paths where the API needs an explicit product set
// (bxgy buy/get sides, code discounts with --target=product); requireScope=false
// lets an empty scope mean "all products" (rebate/mn/flashsale, order-target code).
func resolveScope(productIDs, collectionIDs, variantIDs []string, exclude, requireScope bool, n scopeNames) (map[string]any, error) {
	set := 0
	for _, c := range []int{len(productIDs), len(collectionIDs), len(variantIDs)} {
		if c > 0 {
			set++
		}
	}
	if set > 1 {
		return nil, output.ErrValidation("%s are mutually exclusive; set at most one", n.scopeList())
	}
	if set == 0 {
		if exclude {
			return nil, output.ErrValidation("--%s needs a scope (one of %s); omit --%s to apply to all products", n.exclude, n.scopeList(), n.exclude)
		}
		if requireScope {
			return nil, output.ErrValidation("one of %s is required", n.scopeList())
		}
		return map[string]any{"selection": "all"}, nil
	}
	selection := "entitled"
	if exclude {
		selection = "exclude"
	}
	switch {
	case len(productIDs) > 0:
		return map[string]any{"selection": selection, "product_ids": productIDs}, nil
	case len(collectionIDs) > 0:
		return map[string]any{"selection": selection, "collection_ids": collectionIDs}, nil
	default:
		return map[string]any{"selection": selection, "variant_ids": variantIDs}, nil
	}
}

func allProductsHint(scope map[string]any, n scopeNames) string {
	if scope["selection"] != "all" {
		return ""
	}
	return fmt.Sprintf("applies to every product in the store because no %s was given; pass one to limit it", n.scopeList())
}

// excludeFlag is the standardized --exclude / --get-exclude toggle. name is
// "exclude" (default scope) or "get-exclude" (bxgy get-side). side labels the
// scope in the help text ("" for the primary scope, "get-side " for bxgy get).
func excludeFlag(name, side string) common.Flag {
	return common.Flag{
		Name: name,
		Type: common.FlagBool,
		Description: fmt.Sprintf("Treat the %sscope IDs as a blocklist: the discount applies to all products EXCEPT "+
			"those listed (selection=exclude). Requires one of the scope flags.", side),
	}
}

// GenerateCode returns a random 6-character uppercase alphanumeric discount code.
func GenerateCode() string {
	b := make([]byte, 6)
	for i := range b {
		b[i] = codeChars[rand.Intn(len(codeChars))]
	}
	return string(b)
}

// RebateType describes a rebate's API-side discount/condition/obtain triple.
type RebateType struct {
	DiscountType  string // discount_info.discount_type
	ConditionType string // discount_layer.condition_type
	ObtainType    string // discount_layer.obtain_type
}

// ParseRebateType maps a --type flag value to the full (discount, condition,
// obtain) triple the API expects.
//
// Supported --type values and their mappings:
//
//	amount-off (default) → rebate_cta_otr  purchase_amount    fixed_price_reduction
//	amount-percent       → rebate_cta_otp  purchase_amount    percent
//	qty-off              → rebate_ctq_otr  purchase_quantity  fixed_price_reduction
//	qty-percent          → rebate_ctq_otp  purchase_quantity  percent
//
// Naming key: cta=condition_type=amount, ctq=condition_type=quantity,
// otr=obtain_type=reduction(fixed), otp=obtain_type=percent.
//
// API-validated condition_type values: no_condition, purchase_quantity, purchase_amount
// API-validated obtain_type values: no_discount, free_acquisition, percent,
// fixed_price_reduction, fixed_price, fixed_quantity, product_price_reduction
func ParseRebateType(t string) (RebateType, error) {
	switch t {
	case "amount-off", "":
		return RebateType{"rebate_cta_otr", "purchase_amount", "fixed_price_reduction"}, nil
	case "amount-percent":
		return RebateType{"rebate_cta_otp", "purchase_amount", "percent"}, nil
	case "qty-off":
		return RebateType{"rebate_ctq_otr", "purchase_quantity", "fixed_price_reduction"}, nil
	case "qty-percent":
		return RebateType{"rebate_ctq_otp", "purchase_quantity", "percent"}, nil
	default:
		return RebateType{}, fmt.Errorf("unknown rebate type %q; valid: amount-off, amount-percent, qty-off, qty-percent", t)
	}
}

// ParseFlashsaleType maps a --type flag value to the obtain_type for flashsale.
// Supported: percent (default), fixed-price, off.
func ParseFlashsaleType(t string) (string, error) {
	switch t {
	case "percent", "":
		return "percent", nil
	case "fixed-price":
		return "fixed_price", nil
	case "off":
		return "fixed_price_reduction", nil
	default:
		return "", fmt.Errorf("unknown flashsale type %q; valid: percent, fixed-price, off", t)
	}
}

// activityWindow resolves --start / --end into starts_at / ends_at unix
// seconds; --start defaults to now and ends_at -1 means no expiry.
func activityWindow(in common.PlanInput) (int64, int64, error) {
	start, startSet, err := common.TimeFlag(in, "start")
	if err != nil {
		return 0, 0, err
	}
	if !startSet {
		start = time.Now()
	}
	if !in.Flags.Changed("end") {
		return start.Unix(), -1, nil
	}
	switch strings.ToLower(strings.TrimSpace(in.Flags.GetString("end"))) {
	case "forever", "-1":
		return start.Unix(), -1, nil
	}
	end, _, err := common.TimeFlag(in, "end")
	if err != nil {
		return 0, 0, err
	}
	if end.Unix() <= start.Unix() {
		startLabel := "--start"
		if !startSet {
			startLabel = "the start time, now"
		}
		return 0, 0, output.ErrValidation("--end (%s) must be after %s (%s)",
			end.UTC().Format(time.RFC3339), startLabel, start.UTC().Format(time.RFC3339)).
			WithParam("--end").
			WithSubtype(output.SubtypeInvalidFlagValue)
	}
	return start.Unix(), end.Unix(), nil
}

// buildAutoInfoFromFlags returns a base discount_info map with auto-filled
// common fields (name/display/start/end) for non-code (auto-applied) discounts.
func buildAutoInfoFromFlags(in common.PlanInput) (map[string]any, error) {
	name, err := activityName(in, common.AutoName(in.Tool))
	if err != nil {
		return nil, err
	}
	displayDef := name
	if !in.Flags.Changed("name") {
		displayDef = common.TruncateName(name, maxDisplayNameLen)
	}
	display, err := displayName(in, displayDef, "--name")
	if err != nil {
		return nil, err
	}

	startsAt, endsAt, err := activityWindow(in)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"discount_name": name,
		"display_name":  display,
		"starts_at":     startsAt,
		"ends_at":       endsAt,
	}, nil
}

func planCreateCode(in common.PlanInput, payload map[string]any) common.PlannedRequest {
	p := PlanCreateNonAutomatic(payload)
	if in.Flags.Changed("code") {
		return p
	}
	disc, _ := payload["discount"].(map[string]any)
	info, _ := disc["discount_info"].(map[string]any)
	if codes, _ := info["discount_codes"].([]string); len(codes) == 1 {
		p.DryRunHint = fmt.Sprintf(
			"discount code %s was generated for this preview; a run without --dry-run generates a different one. Pass --code %s to keep it.",
			codes[0], codes[0])
	}
	return p
}

// buildCodeInfoFromFlags returns a base discount_info map for code discounts,
// with auto-generated code when --code is not supplied.
func buildCodeInfoFromFlags(in common.PlanInput) (map[string]any, error) {
	code := GenerateCode()
	if in.Flags.Changed("code") {
		code = in.Flags.GetString("code")
		if n := utf8.RuneCountInString(code); n > maxCodeLen {
			return nil, output.ErrValidation("--code is %d characters; the limit is %d", n, maxCodeLen).
				WithParam("--code").
				WithSubtype(output.SubtypeInvalidFlagValue)
		}
	}
	name, err := activityName(in, code)
	if err != nil {
		return nil, err
	}
	display, err := displayName(in, code, "--code")
	if err != nil {
		return nil, err
	}

	startsAt, endsAt, err := activityWindow(in)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"discount_name":  name,
		"display_name":   display,
		"discount_codes": []string{code},
		"starts_at":      startsAt,
		"ends_at":        endsAt,
	}, nil
}

const (
	maxDiscountNameLen = 50
	maxDisplayNameLen  = 20
	maxCodeLen         = 50
)

func codeFlag() common.Flag {
	return common.Flag{Name: "code", Type: common.FlagString,
		Description: fmt.Sprintf("Discount code, at most %d characters (auto-generated if omitted).", maxCodeLen)}
}

func nameFlag(code bool) common.Flag {
	def := "auto-generated if omitted"
	if code {
		def = "default: the code"
	}
	return common.Flag{Name: "name", Type: common.FlagString,
		Description: fmt.Sprintf("Activity name shown in the admin, at most %d characters (%s).", maxDiscountNameLen, def)}
}

func displayNameFlag(code bool) common.Flag {
	def := fmt.Sprintf("--name; without --name, the generated name cut to %d characters", maxDisplayNameLen)
	if code {
		def = "the code; required when the code is longer"
	}
	return common.Flag{Name: "display-name", Type: common.FlagString,
		Description: fmt.Sprintf("Name buyers see at checkout, at most %d characters (default: %s).", maxDisplayNameLen, def)}
}

func activityName(in common.PlanInput, def string) (string, error) {
	name := def
	if in.Flags.Changed("name") {
		name = in.Flags.GetString("name")
	}
	if n := utf8.RuneCountInString(name); n > maxDiscountNameLen {
		return "", output.ErrValidation("--name is %d characters; the limit is %d", n, maxDiscountNameLen).
			WithParam("--name").
			WithSubtype(output.SubtypeInvalidFlagValue)
	}
	return name, nil
}

func displayName(in common.PlanInput, def, defFlag string) (string, error) {
	if in.Flags.Changed("display-name") {
		display := in.Flags.GetString("display-name")
		if n := utf8.RuneCountInString(display); n > maxDisplayNameLen {
			return "", output.ErrValidation("--display-name is %d characters; buyers see at most %d", n, maxDisplayNameLen).
				WithParam("--display-name").
				WithSubtype(output.SubtypeInvalidFlagValue)
		}
		return display, nil
	}
	if n := utf8.RuneCountInString(def); n > maxDisplayNameLen {
		return "", output.ErrValidation("--display-name is required: the display name buyers see defaults to %s, which is %d characters; the limit is %d", defFlag, n, maxDisplayNameLen).
			WithParam("--display-name").
			WithSubtype(output.SubtypeMissingRequiredFlag).
			WithHint(fmt.Sprintf("pass --display-name with at most %d characters", maxDisplayNameLen))
	}
	return def, nil
}

// validateCombines reads --combines, validates each entry, and returns a
// non-nil slice (empty if unset, so JSON encodes as [] not null).
func validateCombines(in common.PlanInput) ([]string, error) {
	combines := in.Flags.GetStringSlice("combines")
	for _, c := range combines {
		if c != "order" && c != "product" && c != "shipping" {
			return nil, output.ErrValidation("--combines value %q invalid; allowed: order, product, shipping", c)
		}
	}
	if combines == nil {
		combines = []string{}
	}
	return combines, nil
}

// codeRuleFromFlags produces the discount_rule map from --combines /
// --limit-max / --limit-user. --limit-max and --limit-user are optional;
// omitting either is equivalent to -1 (no limit). Other limit_* fields are
// left out — the server applies its own defaults.
func codeRuleFromFlags(in common.PlanInput) (map[string]any, error) {
	combines, err := validateCombines(in)
	if err != nil {
		return nil, err
	}
	limitMax, limitUser, err := resolveLimitMaxUser(in)
	if err != nil {
		return nil, err
	}
	return map[string]any{
		"discount_combines":   combines,
		"limit_max_discount":  limitMax,
		"limit_user_discount": limitUser,
	}, nil
}

// codeOffFlags returns the shared flag set for +percent-code and +amount-code.
// The value flag is shortcut-specific (--percent-off for percent-code, --off for
// amount-code — they mean different things) and is declared in each shortcut.
func codeOffFlags() []common.Flag {
	return []common.Flag{
		{Name: "target", Type: common.FlagString, Required: true,
			Description: "Discount scope: order or product (required).",
			Completions: []string{"order", "product"}},
		{Name: "products", Type: common.FlagStringSlice, Description: "Product IDs comma-separated (--target=product only; mutex with --variants / --collections)."},
		{Name: "variants", Type: common.FlagStringSlice, Description: "Variant IDs comma-separated (--target=product only; mutex with --products / --collections)."},
		{Name: "collections", Type: common.FlagStringSlice, Description: "Collection IDs comma-separated (--target=product only; mutex with --products / --variants)."},
		excludeFlag("exclude", ""),
		{Name: "min-amount", Type: common.FlagFloat, Description: "Minimum order amount (--target=order only; rejected with --target=product)."},
		{Name: "min-quantity", Type: common.FlagInt, Default: 1, Description: "Minimum item count (--target=product only; rejected with --target=order; default 1)."},
		codeFlag(),
		nameFlag(true),
		displayNameFlag(true),
		common.StartTimeFlag(),
		common.EndTimeFlag(),
		{Name: "combines", Type: common.FlagStringSlice,
			Description: "Allowed combinations (subset of order/product/shipping; default: [] = no stacking).",
			Completions: []string{"order", "product", "shipping"}},
		{Name: "limit-max", Type: common.FlagInt, Description: "Max total discount uses across all customers (>0; omit for no limit)."},
		{Name: "limit-user", Type: common.FlagInt, Description: "Max discount uses per customer (>0; omit for no limit)."},
		customerSegmentsFlag(),
	}
}

// validateLayerObtainValues enforces a numeric range on every layer.ObtainValue.
// When isPercent is true the bound is [1, 99]; otherwise just > 0. allowZero
// lowers the floor to 0 (percent [0, 99], amount >= 0) for quantity-threshold
// rebates, which must accept a 0-discount tier (e.g. "1:0"). Returns the first
// failure as a --tiers validation error pointing at the offending tier.
func validateLayerObtainValues(layers []common.Layer, isPercent, allowZero bool) error {
	for _, l := range layers {
		if isPercent {
			floor := 1.0
			if allowZero {
				floor = 0
			}
			if l.ObtainValue < floor || l.ObtainValue > 99 {
				return output.ErrValidation("--tiers: percent must be %v-99 (got %v in tier %v:%v)", floor, l.ObtainValue, l.ConditionValue, l.ObtainValue)
			}
			continue
		}
		if allowZero && l.ObtainValue < 0 {
			return output.ErrValidation("--tiers: discount must be >= 0 (got %v in tier %v:%v)", l.ObtainValue, l.ConditionValue, l.ObtainValue)
		}
		if !allowZero && l.ObtainValue <= 0 {
			return output.ErrValidation("--tiers: discount must be > 0 (got %v in tier %v:%v)", l.ObtainValue, l.ConditionValue, l.ObtainValue)
		}
	}
	return nil
}

// resolveLimitMaxUser reads --limit-max / --limit-user; unset → -1 (no limit),
// set → must be > 0. Used by every discount shortcut that exposes these two
// campaign-wide usage caps (i.e. all of them except +flashsale, which drops
// the concept entirely).
func resolveLimitMaxUser(in common.PlanInput) (limitMax, limitUser int, err error) {
	limitMax = -1
	limitUser = -1
	if in.Flags.Changed("limit-max") {
		limitMax = in.Flags.GetInt("limit-max")
		if limitMax <= 0 {
			return 0, 0, output.ErrValidation("--limit-max must be > 0 (got %d)", limitMax)
		}
	}
	if in.Flags.Changed("limit-user") {
		limitUser = in.Flags.GetInt("limit-user")
		if limitUser <= 0 {
			return 0, 0, output.ErrValidation("--limit-user must be > 0 (got %d)", limitUser)
		}
	}
	return limitMax, limitUser, nil
}

// buildCodeDiscountPayload assembles the non-automatic POST body for the
// +percent-code / +amount-code shortcuts. discountType / obtainType differ
// per shortcut; the target-driven branching is shared:
//
//	target=order   → entitled_product:{}, condition by --min-amount (purchase_amount).
//	target=product → entitled_product via --products / --variants / --collections
//	                 (exactly one required), condition by --min-quantity (purchase_quantity).
func buildCodeDiscountPayload(in common.PlanInput, discountType, obtainType string, obtainValue float64) (map[string]any, error) {
	target := in.Flags.GetString("target")
	if target != "order" && target != "product" {
		return nil, output.ErrValidation("--target must be 'order' or 'product' (got %q)", target)
	}
	info, err := buildCodeInfoFromFlags(in)
	if err != nil {
		return nil, err
	}
	info["discount_type"] = discountType
	info["discount_target"] = target

	var (
		condType, condValue string
		entitledProduct     map[string]any
	)
	if target == "order" {
		// Order-level discount applies to the whole order: product scope is not
		// honored, --exclude is product-level only, and entitled_product is {}.
		if in.Flags.GetBool("exclude") {
			return nil, output.ErrValidation("--exclude only applies to --target=product")
		}
		var offTarget []string
		for _, name := range []string{"products", "variants", "collections", "min-quantity"} {
			if in.Flags.Changed(name) {
				offTarget = append(offTarget, "--"+name)
			}
		}
		if len(offTarget) > 0 {
			verb := "applies"
			if len(offTarget) > 1 {
				verb = "apply"
			}
			return nil, output.ErrValidation("%s only %s to --target=product", strings.Join(offTarget, ", "), verb).
				WithParam(offTarget[0]).
				WithHint("for a discount on specific products or with a minimum item count, use --target=product; for the whole order, drop these flags")
		}
		entitledProduct = map[string]any{}
		min := in.Flags.GetFloat("min-amount")
		condType = "no_condition"
		if min > 0 {
			condType = "purchase_amount"
		}
		condValue = strconv.FormatFloat(min, 'f', -1, 64)
	} else {
		if in.Flags.Changed("min-amount") {
			return nil, output.ErrValidation("--min-amount only applies to --target=order").
				WithParam("--min-amount").
				WithHint("for a per-product threshold use --min-quantity; for a minimum order amount, use --target=order")
		}
		scope, err := resolveScope(
			common.IDList(in.Flags, "products"),
			common.IDList(in.Flags, "collections"),
			common.IDList(in.Flags, "variants"),
			in.Flags.GetBool("exclude"), true, defaultScopeNames())
		if err != nil {
			return nil, err
		}
		entitledProduct = scope
		minQty := in.Flags.GetInt("min-quantity")
		if minQty < 1 {
			minQty = 1
		}
		condType = "no_condition"
		if minQty > 1 {
			condType = "purchase_quantity"
		}
		condValue = strconv.Itoa(minQty)
	}

	rule, err := codeRuleFromFlags(in)
	if err != nil {
		return nil, err
	}

	return map[string]any{
		"discount": map[string]any{
			"discount_info": info,
			"discount_layer": map[string]any{
				"condition_type": condType,
				"obtain_type":    obtainType,
				"layers": []any{map[string]any{
					"condition_value": condValue,
					"obtain_value":    strconv.FormatFloat(obtainValue, 'f', -1, 64),
				}},
			},
			"discount_rule":     rule,
			"entitled_customer": resolveEntitledCustomer(in),
			"entitled_product":  entitledProduct,
		},
	}, nil
}
