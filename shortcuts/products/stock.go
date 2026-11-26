package products

import (
	"context"
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// stockShortcut wires +stock to inventory writes.
//
// Increases ride PUT /inventory_levels (stock_adjustment > 0; the API rejects ≤ 0).
// Decreases ride POST /inventory_levels/set, which despite its name adds its
// signed stock to the level at the given location.
var stockShortcut = common.Shortcut{
	Service: "products",
	Command: "+stock",
	Use:     "+stock (--variant-id <id> | --sku <sku> | --product-id <id>) (--set <n> | --adjust <±n>) [--location-id <id>]",
	Short:   "Set or adjust variant inventory level",
	Long:    "Set an absolute stock level or apply a delta for a variant (by variant ID, SKU, or product ID) at any location; stock may go below 0. Run --dry-run first to preview.",
	Example: `  # Preview restocking a SKU by +50
  shoplazza products +stock --sku TS-RED-M --adjust 50 --dry-run

  # Set an absolute stock level by variant ID
  shoplazza products +stock --variant-id v-1 --set 200`,
	Flags: []common.Flag{
		{Name: "variant-id", Type: common.FlagString, Description: "Variant ID — the unique, exact target."},
		{Name: "sku", Type: common.FlagString, Description: "Variant SKU. Resolves to one variant; a multi-match is refused with the candidates listed (there is no --all here: inventory writes target one variant)."},
		{Name: "product-id", Type: common.FlagString, Description: "Product ID. Resolves to that product's single variant; a multi-variant product is refused with the candidates listed."},
		{Name: "set", Type: common.FlagInt, Description: "Set inventory to an absolute target (may be negative). Mutually exclusive with --adjust."},
		{Name: "adjust", Type: common.FlagInt, Description: "Stock delta (nonzero): positive adds, negative subtracts, and stock may go below 0. Mutex with --set."},
		{Name: "location-id", Type: common.FlagString, Description: "Location ID (defaults to default location)."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		target, err := parseVariantTarget(in)
		if err != nil {
			return common.ExecResult{}, err
		}
		locationID := in.Flags.GetString("location-id")

		gotSet := in.Flags.Changed("set")
		gotAdjust := in.Flags.Changed("adjust")
		if gotSet && gotAdjust {
			return common.ExecResult{}, output.ErrValidation("--set and --adjust are mutually exclusive")
		}
		if !gotSet && !gotAdjust {
			return common.ExecResult{}, output.ErrValidation("one of --set or --adjust is required")
		}

		if gotAdjust && in.Flags.GetInt("adjust") == 0 {
			return common.ExecResult{}, output.ErrValidation("--adjust must be nonzero (positive adds, negative subtracts)")
		}

		// Resolve after the pure gates: a bad --set/--adjust must not cost a network
		// call. base = plans preceding the write steps, keeping their step refs correct.
		variantID, base := target.VariantID, 0
		var resolvePlan common.PlannedRequest
		if target.NeedsResolve() {
			resolvePlan, base = target.ResolvePlan(), 1
			if in.DryRun {
				variantID = stepRef(0)
			} else if variantID, err = target.Resolve(ctx, in.Client); err != nil {
				return common.ExecResult{}, err
			}
		}

		var res common.ExecResult
		if gotAdjust {
			res, err = execStockAdjust(ctx, in, variantID, locationID, in.Flags.GetInt("adjust"), base)
		} else {
			res, err = execStockSet(ctx, in, variantID, locationID, base)
		}
		if err != nil {
			return common.ExecResult{}, target.hintIfDirectVariantID(err)
		}
		if in.DryRun && base > 0 {
			res.Plans = append([]common.PlannedRequest{resolvePlan}, res.Plans...)
		}
		return res, nil
	},
}

func execStockAdjust(ctx context.Context, in common.ExecInput, variantID, locationID string, adjust, base int) (common.ExecResult, error) {
	invPlan := PlanInventoryItemForVariant(variantID)
	plans := []common.PlannedRequest{invPlan}

	var locPlan common.PlannedRequest
	needsDefaultLoc := locationID == ""
	if needsDefaultLoc {
		locPlan = PlanDefaultLocation()
		plans = append(plans, locPlan)
	}
	plans = append(plans,
		PlanListItemLevels(stepRef(base+0)),
		PlanGetInventoryItem(stepRef(base+0)),
		planStockDelta(stepRef(base+0), placeholderOr(locationID, stepRef(base+1)), adjust),
	)

	if in.DryRun {
		return common.ExecResult{Plans: plans}, nil
	}

	invItemID, err := resolveInventoryItemID(ctx, in.Client, invPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	if needsDefaultLoc {
		locationID, err = resolveDefaultLocationID(ctx, in.Client, locPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
	}
	if _, err := stockedLevel(ctx, in.Client, invItemID, locationID); err != nil {
		return common.ExecResult{}, err
	}
	if err := requireTracking(ctx, in.Client, invItemID); err != nil {
		return common.ExecResult{}, err
	}

	resp, err := common.Send(ctx, in.Client, planStockDelta(invItemID, locationID, adjust))
	if err != nil {
		return common.ExecResult{}, err
	}
	return common.ExecResult{Body: resp}, nil
}

func execStockSet(ctx context.Context, in common.ExecInput, variantID, locationID string, base int) (common.ExecResult, error) {
	invPlan := PlanInventoryItemForVariant(variantID)
	plans := []common.PlannedRequest{invPlan}

	var locPlan common.PlannedRequest
	needsDefaultLoc := locationID == ""
	if needsDefaultLoc {
		locPlan = PlanDefaultLocation()
		plans = append(plans, locPlan)
	}
	effectiveLoc := placeholderOr(locationID, stepRef(base+1))
	plans = append(plans,
		PlanListItemLevels(stepRef(base+0)),
		PlanGetInventoryItem(stepRef(base+0)),
		PlanAdjustInventoryLevel(map[string]any{
			"inventory_item_id": stepRef(base + 0),
			"location_id":       effectiveLoc,
			"stock_adjustment":  "<if target > current: target minus current>",
		}),
		PlanSetStock(map[string]any{
			"inventory_item_id": stepRef(base + 0),
			"location_id":       effectiveLoc,
			"stock":             "<if target < current: target minus current>",
		}),
	)

	if in.DryRun {
		return common.ExecResult{Plans: plans}, nil
	}

	invItemID, err := resolveInventoryItemID(ctx, in.Client, invPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	if needsDefaultLoc {
		locationID, err = resolveDefaultLocationID(ctx, in.Client, locPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
	}

	row, err := stockedLevel(ctx, in.Client, invItemID, locationID)
	if err != nil {
		return common.ExecResult{}, err
	}
	current, err := stockOf(row)
	if err != nil {
		return common.ExecResult{}, err
	}
	if err := requireTracking(ctx, in.Client, invItemID); err != nil {
		return common.ExecResult{}, err
	}

	target := in.Flags.GetInt("set")
	if target == current {
		return common.ExecResult{Body: wrapLevelRow(row)}, nil
	}
	resp, err := common.Send(ctx, in.Client, planStockDelta(invItemID, locationID, target-current))
	if err != nil {
		return common.ExecResult{}, err
	}
	return common.ExecResult{Body: resp}, nil
}

func planStockDelta(invItemID, locationID string, delta int) common.PlannedRequest {
	if delta > 0 {
		return PlanAdjustInventoryLevel(map[string]any{
			"inventory_item_id": invItemID,
			"location_id":       locationID,
			"stock_adjustment":  delta,
		})
	}
	return PlanSetStock(map[string]any{
		"inventory_item_id": invItemID,
		"location_id":       locationID,
		"stock":             delta,
	})
}

// resolveInventoryItemID runs the variant→inventory-item lookup plan and extracts the id.
func resolveInventoryItemID(ctx context.Context, c *client.Client, plan common.PlannedRequest) (string, error) {
	resp, err := common.Send(ctx, c, plan)
	if err != nil {
		return "", err
	}
	return extractInventoryItemID(resp)
}

func stockedLevel(ctx context.Context, c *client.Client, invItemID, locationID string) (map[string]any, error) {
	resp, err := common.Send(ctx, c, PlanListItemLevels(invItemID))
	if err != nil {
		return nil, err
	}
	row, err := levelRowFor(resp, locationID)
	if err != nil || row != nil {
		return row, err
	}
	var stocked []string
	rows, _ := resp["inventory_levels"].([]any)
	for _, r := range rows {
		if m, ok := r.(map[string]any); ok {
			stocked = append(stocked, asString(m["location_id"]))
		}
	}
	return nil, output.ErrValidation("this variant is not stocked at location %s (stocked at: %s)", locationID, strings.Join(stocked, ", ")).
		WithParam("--location-id").
		WithHint(fmt.Sprintf(`pick one of those with --location-id, or connect it first: products inventory create-level --data '{"inventory_item_id":"%s","location_id":"%s"}'`, invItemID, locationID))
}

func requireTracking(ctx context.Context, c *client.Client, invItemID string) error {
	resp, err := common.Send(ctx, c, PlanGetInventoryItem(invItemID))
	if err != nil {
		return err
	}
	item, ok := resp["inventory_item"].(map[string]any)
	if !ok {
		return output.ErrInternal("inventory item response missing 'inventory_item' object")
	}
	if tracking, _ := item["tracking"].(bool); !tracking {
		return output.ErrValidation("this variant's product does not track inventory, so its stock cannot be changed").
			WithHint("turn on the product's inventory tracking first ('products update' with product.inventory_tracking: true), then run +stock again")
	}
	return nil
}

// resolveDefaultLocationID runs the default-location lookup plan and extracts the id.
func resolveDefaultLocationID(ctx context.Context, c *client.Client, plan common.PlannedRequest) (string, error) {
	resp, err := common.Send(ctx, c, plan)
	if err != nil {
		return "", err
	}
	return extractDefaultLocationID(resp)
}

// placeholderOr returns v, or the placeholder when v is empty.
func placeholderOr(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}

// levelRowFor picks locationID's row out of a levels list response. A missing row is (nil, nil).
func levelRowFor(resp map[string]any, locationID string) (map[string]any, error) {
	rows, ok := resp["inventory_levels"].([]any)
	if !ok {
		return nil, output.ErrInternal("inventory_levels response missing 'inventory_levels' array")
	}
	var match map[string]any
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, output.ErrInternal("inventory_levels row not an object")
		}
		if asString(row["location_id"]) == locationID {
			match = row
		}
	}
	return match, nil
}

// stockOf reads a row's stock; the API omits the field when it is 0.
func stockOf(row map[string]any) (int, error) {
	raw, present := row["stock"]
	if !present {
		return 0, nil
	}
	n, ok := asInt(raw)
	if !ok {
		return 0, output.ErrInternal("inventory_level.stock has unexpected type")
	}
	return n, nil
}

// wrapLevelRow adapts a level row into the {"inventory_level": {...}} shape PUT returns.
func wrapLevelRow(row map[string]any) map[string]any {
	if row == nil {
		row = map[string]any{}
	}
	return map[string]any{"inventory_level": row}
}

// asInt converts a JSON-decoded numeric value to int.
func asInt(v any) (int, bool) {
	switch x := v.(type) {
	case json.Number:
		n, err := x.Int64()
		if err != nil {
			return 0, false
		}
		return int(n), true
	case float64:
		return int(x), true
	case int:
		return x, true
	case int64:
		return int(x), true
	default:
		return 0, false
	}
}

// noInventoryItemMsg prefixes the empty variant→item lookup — a mistyped id, not a
// CLI bug, hence validation-class. hintIfDirectVariantID matches on it.
const noInventoryItemMsg = "no inventory item found for that variant"

func extractInventoryItemID(resp map[string]any) (string, error) {
	items, ok := resp["variant_inventory_items"].([]any)
	if !ok || len(items) == 0 {
		return "", output.ErrValidation("%s — the variant id matched nothing", noInventoryItemMsg)
	}
	m, ok := items[0].(map[string]any)
	if !ok {
		return "", output.ErrInternal("variant_inventory_items[0] not an object")
	}
	id := asString(m["inventory_item_id"])
	if id == "" {
		return "", output.ErrInternal("variant_inventory_items[0].inventory_item_id missing")
	}
	return id, nil
}

func extractDefaultLocationID(resp map[string]any) (string, error) {
	loc, ok := resp["location"].(map[string]any)
	if !ok {
		return "", output.ErrInternal("default location response missing 'location' object")
	}
	id := asString(loc["id"])
	if id == "" {
		return "", output.ErrInternal("default location.id missing")
	}
	return id, nil
}

// asString normalizes a JSON value (string, json.Number, float64, or int) to its decimal string form.
// Large numeric IDs arrive as json.Number (decoded with UseNumber) to preserve their exact value beyond 2^53.
func asString(v any) string {
	switch x := v.(type) {
	case string:
		return x
	case json.Number:
		return x.String()
	case float64:
		if x == float64(int64(x)) {
			return strconv.FormatInt(int64(x), 10)
		}
		return strconv.FormatFloat(x, 'f', -1, 64)
	case int:
		return strconv.Itoa(x)
	case int64:
		return strconv.FormatInt(x, 10)
	default:
		return ""
	}
}
