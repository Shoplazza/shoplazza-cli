package products

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// stockShortcut wires +stock to inventory writes.
//
// Increases ride PUT /inventory_levels (stock_adjustment > 0; the API rejects ≤ 0).
// Decreases ride PUT /variants/{id} with inventory_quantity, which the platform
// treats as an absolute set. That write lands on the default location, so
// decrements are gated: default location only, single-location items only, and
// the target must stay ≥ 0 (the API happily stores negatives).
var stockShortcut = common.Shortcut{
	Service: "products",
	Command: "+stock",
	Use:     "+stock --variant-id <id> (--set <n> | --adjust <±n>) [--location-id <id>]",
	Short:   "Set or adjust variant inventory level",
	Flags: []common.Flag{
		{Name: "variant-id", Type: common.FlagString, Required: true, Description: "Variant ID (required)."},
		{Name: "set", Type: common.FlagInt, Description: "Set inventory to an absolute target (≥ 0). Decreases work only at the default location of a single-location item. Mutually exclusive with --adjust."},
		{Name: "adjust", Type: common.FlagInt, Description: "Stock delta (nonzero). Positive adds at any location; negative decreases (same gates as --set). Mutex with --set."},
		{Name: "location-id", Type: common.FlagString, Description: "Location ID (defaults to default location)."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		variantID := in.Flags.GetString("variant-id")
		locationID := in.Flags.GetString("location-id")

		gotSet := in.Flags.Changed("set")
		gotAdjust := in.Flags.Changed("adjust")
		if gotSet && gotAdjust {
			return common.ExecResult{}, output.ErrValidation("--set and --adjust are mutually exclusive")
		}
		if !gotSet && !gotAdjust {
			return common.ExecResult{}, output.ErrValidation("one of --set or --adjust is required")
		}

		if gotSet && in.Flags.GetInt("set") < 0 {
			return common.ExecResult{}, output.ErrValidation("--set must be ≥ 0, got %d", in.Flags.GetInt("set"))
		}
		if gotAdjust && in.Flags.GetInt("adjust") == 0 {
			return common.ExecResult{}, output.ErrValidation("--adjust must be nonzero (positive adds, negative decreases)")
		}

		adjust := in.Flags.GetInt("adjust")
		if gotAdjust && adjust > 0 {
			return execStockAdd(ctx, in, variantID, locationID, adjust)
		}
		return execStockSetOrDecrease(ctx, in, variantID, locationID, gotSet)
	},
}

// execStockAdd is the fast path for --adjust > 0: no reads beyond id resolution.
func execStockAdd(ctx context.Context, in common.ExecInput, variantID, locationID string, adjust int) (common.ExecResult, error) {
	plans := []common.PlannedRequest{}
	invPlan := PlanInventoryItemForVariant(variantID)
	plans = append(plans, invPlan)

	var locPlan common.PlannedRequest
	needsDefaultLoc := locationID == ""
	if needsDefaultLoc {
		locPlan = PlanDefaultLocation()
		plans = append(plans, locPlan)
	}

	previewBody := map[string]any{
		"inventory_item_id": "<resolved-from-step-0>",
		"location_id":       placeholderOr(locationID, "<resolved-from-step-1>"),
		"stock_adjustment":  adjust,
	}
	plans = append(plans, PlanAdjustInventoryLevel(previewBody))

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

	resp, err := common.Send(ctx, in.Client, PlanAdjustInventoryLevel(map[string]any{
		"inventory_item_id": invItemID,
		"location_id":       locationID,
		"stock_adjustment":  adjust,
	}))
	if err != nil {
		return common.ExecResult{}, translateAdjustError(err)
	}
	return common.ExecResult{Body: resp}, nil
}

// execStockSetOrDecrease handles --set and --adjust < 0: read the current level,
// then route up (inventory_levels add) or down (variant inventory_quantity set).
func execStockSetOrDecrease(ctx context.Context, in common.ExecInput, variantID, locationID string, gotSet bool) (common.ExecResult, error) {
	invPlan := PlanInventoryItemForVariant(variantID)
	locPlan := PlanDefaultLocation() // always: decrement gating compares against it
	levelsPlan := PlanListItemLevels("<resolved-from-step-0>")
	plans := []common.PlannedRequest{invPlan, locPlan, levelsPlan}

	effectiveLoc := placeholderOr(locationID, "<resolved-from-step-1>")
	if gotSet {
		// Direction is unknown until the level is read: preview both writes.
		target := in.Flags.GetInt("set")
		plans = append(plans,
			PlanAdjustInventoryLevel(map[string]any{
				"inventory_item_id": "<resolved-from-step-0>",
				"location_id":       effectiveLoc,
				"stock_adjustment":  "<if target > current: target minus current>",
			}),
			PlanUpdateVariant(variantID, map[string]any{
				"variant": map[string]any{"inventory_quantity": target},
			}),
		)
	} else {
		plans = append(plans, PlanUpdateVariant(variantID, map[string]any{
			"variant": map[string]any{"inventory_quantity": "<computed: current minus |adjust|>"},
		}))
	}

	if in.DryRun {
		return common.ExecResult{Plans: plans}, nil
	}

	invItemID, err := resolveInventoryItemID(ctx, in.Client, invPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	defaultLoc, err := resolveDefaultLocationID(ctx, in.Client, locPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	if locationID == "" {
		locationID = defaultLoc
	}

	levelsResp, err := common.Send(ctx, in.Client, PlanListItemLevels(invItemID))
	if err != nil {
		return common.ExecResult{}, err
	}
	row, locCount, err := levelRowFor(levelsResp, locationID)
	if err != nil {
		return common.ExecResult{}, err
	}
	current := 0
	if row != nil {
		if current, err = stockOf(row); err != nil {
			return common.ExecResult{}, err
		}
	}

	target := in.Flags.GetInt("set")
	if !gotSet {
		target = current + in.Flags.GetInt("adjust")
	}

	switch {
	case target == current:
		return common.ExecResult{Body: wrapLevelRow(row)}, nil
	case target > current:
		resp, aerr := common.Send(ctx, in.Client, PlanAdjustInventoryLevel(map[string]any{
			"inventory_item_id": invItemID,
			"location_id":       locationID,
			"stock_adjustment":  target - current,
		}))
		if aerr != nil {
			return common.ExecResult{}, translateAdjustError(aerr)
		}
		return common.ExecResult{Body: resp}, nil
	}

	// Decrement path: variant.inventory_quantity is an absolute set that lands
	// on the default location, so gate anything it cannot express safely.
	if target < 0 {
		return common.ExecResult{}, output.ErrValidation(
			"--adjust %d would take stock below 0 (current=%d); use --set 0 to zero it out", in.Flags.GetInt("adjust"), current)
	}
	if locationID != defaultLoc {
		return common.ExecResult{}, output.ErrValidation(
			"stock decrease writes variant.inventory_quantity, which only targets the default location (%s); got --location-id %s", defaultLoc, locationID)
	}
	if locCount > 1 {
		return common.ExecResult{}, output.ErrValidation(
			"stock decrease is unsupported for items stocked at multiple locations (%d found): variant.inventory_quantity semantics are only verified for single-location items", locCount)
	}

	if _, err = common.Send(ctx, in.Client, PlanUpdateVariant(variantID, map[string]any{
		"variant": map[string]any{"inventory_quantity": target},
	})); err != nil {
		return common.ExecResult{}, err
	}

	// Re-read so the caller gets the same {"inventory_level": …} shape as adds.
	afterResp, err := common.Send(ctx, in.Client, PlanListItemLevels(invItemID))
	if err != nil {
		return common.ExecResult{}, err
	}
	afterRow, _, err := levelRowFor(afterResp, locationID)
	if err != nil {
		return common.ExecResult{}, err
	}
	return common.ExecResult{Body: wrapLevelRow(afterRow)}, nil
}


// resolveInventoryItemID runs the variant→inventory-item lookup plan and extracts the id.
func resolveInventoryItemID(ctx context.Context, c *client.Client, plan common.PlannedRequest) (string, error) {
	resp, err := common.Send(ctx, c, plan)
	if err != nil {
		return "", err
	}
	return extractInventoryItemID(resp)
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

// levelRowFor picks locationID's row out of a levels list response and reports
// how many locations hold a level. A missing row is (nil, n, nil).
func levelRowFor(resp map[string]any, locationID string) (map[string]any, int, error) {
	rows, ok := resp["inventory_levels"].([]any)
	if !ok {
		return nil, 0, output.ErrInternal("inventory_levels response missing 'inventory_levels' array")
	}
	var match map[string]any
	for _, r := range rows {
		row, ok := r.(map[string]any)
		if !ok {
			return nil, 0, output.ErrInternal("inventory_levels row not an object")
		}
		if asString(row["location_id"]) == locationID {
			match = row
		}
	}
	return match, len(rows), nil
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

func extractInventoryItemID(resp map[string]any) (string, error) {
	items, ok := resp["variant_inventory_items"].([]any)
	if !ok || len(items) == 0 {
		return "", output.ErrInternal("variant_inventory_items lookup returned empty array")
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

// translateAdjustError converts 422 rejections (typically "stock would go negative") into ErrValidation;
// other errors pass through unchanged.
func translateAdjustError(err error) error {
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) {
		return err
	}
	if httpErr.StatusCode != 422 {
		return err
	}
	body := httpErr.Body
	if strings.Contains(body, `"current_stock"`) {
		return output.ErrValidation("inventory adjustment rejected (resulting stock would be < 0). API said: %s", body)
	}
	return output.ErrValidation("inventory adjustment rejected by API: %s", body)
}
