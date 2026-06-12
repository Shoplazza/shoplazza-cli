package products

import (
	"context"
	"encoding/json"
	"errors"
	"strconv"
	"strings"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

// stockShortcut wires +stock to inventory writes.
//
// --adjust N: direct PUT /inventory_levels with stock_adjustment=N (N must be > 0; the API rejects 0 and negatives).
// --set N: client-side simulation — GET the current level, compute delta = N - current, then PUT, since the
// /set endpoint behaves as add (not set). Decrement (N < current) is rejected because the API has no decrement primitive.
var stockShortcut = common.Shortcut{
	Service: "products",
	Command: "+stock",
	Use:     "+stock --variant-id <id> (--set <n> | --adjust <+n>) [--location-id <id>]",
	Short:   "Set or adjust variant inventory level",
	Flags: []common.Flag{
		{Name: "variant-id", Type: common.FlagString, Required: true, Description: "Variant ID (required)."},
		{Name: "set", Type: common.FlagInt, Description: "Set inventory to an absolute value (≥ 0; mutex with --adjust). Implemented client-side as GET current + PUT delta; decrement is rejected because the API does not support negative stock_adjustment."},
		{Name: "adjust", Type: common.FlagInt, Description: "Stock delta to add (> 0; the API rejects 0 and negative values). Mutex with --set."},
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
		if gotAdjust && in.Flags.GetInt("adjust") <= 0 {
			return common.ExecResult{}, output.ErrValidation("--adjust must be > 0 (got %d); the API rejects 0 and negative adjustments.", in.Flags.GetInt("adjust"))
		}

		plans := []common.PlannedRequest{}
		invPlan := PlanInventoryItemForVariant(variantID)
		plans = append(plans, invPlan)

		var locPlan common.PlannedRequest
		needsDefaultLoc := locationID == ""
		if needsDefaultLoc {
			locPlan = PlanDefaultLocation()
			plans = append(plans, locPlan)
		}

		// For --set we also list the existing inventory_level to compute the
		// adjustment. Use placeholders in the dry-run preview body.
		var getLevelPlan common.PlannedRequest
		if gotSet {
			getLevelPlan = PlanGetInventoryLevel("<resolved-from-step-0>", placeholderOr(locationID, "<resolved-from-step-1>"))
			plans = append(plans, getLevelPlan)
		}

		previewBody := map[string]any{
			"inventory_item_id": "<resolved-from-step-0>",
			"location_id":       placeholderOr(locationID, "<resolved-from-step-1>"),
		}
		if gotSet {
			previewBody["stock_adjustment"] = "<computed: --set N minus current>"
		} else {
			previewBody["stock_adjustment"] = in.Flags.GetInt("adjust")
		}
		plans = append(plans, PlanAdjustInventoryLevel(previewBody))

		if in.DryRun {
			return common.ExecResult{Plans: plans}, nil
		}

		invResp, err := common.Send(ctx, in.Client, invPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		invItemID, err := extractInventoryItemID(invResp)
		if err != nil {
			return common.ExecResult{}, err
		}
		if needsDefaultLoc {
			locResp, lerr := common.Send(ctx, in.Client, locPlan)
			if lerr != nil {
				return common.ExecResult{}, lerr
			}
			locationID, err = extractDefaultLocationID(locResp)
			if err != nil {
				return common.ExecResult{}, err
			}
		}

		var delta int
		if gotSet {
			target := in.Flags.GetInt("set")
			levelResp, lerr := common.Send(ctx, in.Client, PlanGetInventoryLevel(invItemID, locationID))
			if lerr != nil {
				return common.ExecResult{}, lerr
			}
			current, cerr := extractInventoryLevelStock(levelResp)
			if cerr != nil {
				return common.ExecResult{}, cerr
			}
			delta = target - current
			if delta == 0 {
				// No-op: return the current level shape so the caller still sees
				// {"inventory_level": {...}}; wrap the single level row from the
				// list response.
				return common.ExecResult{Body: wrapSingleLevel(levelResp)}, nil
			}
			if delta < 0 {
				return common.ExecResult{}, output.ErrValidation(
					"--set %d would decrement from current=%d by %d, but the API does not support stock reduction (PUT /inventory_levels rejects stock_adjustment ≤ 0). Use --adjust to increase, or wait for backend to expose a decrement endpoint.",
					target, current, current-target)
			}
		} else {
			delta = in.Flags.GetInt("adjust")
		}

		liveBody := map[string]any{
			"inventory_item_id": invItemID,
			"location_id":       locationID,
			"stock_adjustment":  delta,
		}
		resp, err := common.Send(ctx, in.Client, PlanAdjustInventoryLevel(liveBody))
		if err != nil {
			return common.ExecResult{}, translateAdjustError(err)
		}
		return common.ExecResult{Body: resp}, nil
	},
}

// placeholderOr returns v, or the placeholder when v is empty.
func placeholderOr(v, placeholder string) string {
	if v == "" {
		return placeholder
	}
	return v
}

// extractInventoryLevelStock pulls the stock value out of a GET /inventory_levels response.
// A missing `stock` field is treated as 0, since the API omits it when the value is 0.
func extractInventoryLevelStock(resp map[string]any) (int, error) {
	rows, ok := resp["inventory_levels"].([]any)
	if !ok {
		return 0, output.ErrInternal("inventory_levels response missing 'inventory_levels' array")
	}
	if len(rows) == 0 {
		return 0, nil
	}
	row, ok := rows[0].(map[string]any)
	if !ok {
		return 0, output.ErrInternal("inventory_levels[0] not an object")
	}
	if raw, present := row["stock"]; present {
		if n, ok := asInt(raw); ok {
			return n, nil
		}
		return 0, output.ErrInternal("inventory_levels[0].stock has unexpected type")
	}
	return 0, nil
}

// wrapSingleLevel adapts a GET /inventory_levels list response into the {"inventory_level": {...}} shape PUT returns.
func wrapSingleLevel(listResp map[string]any) map[string]any {
	rows, _ := listResp["inventory_levels"].([]any)
	if len(rows) == 0 {
		return map[string]any{"inventory_level": map[string]any{}}
	}
	row, _ := rows[0].(map[string]any)
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
