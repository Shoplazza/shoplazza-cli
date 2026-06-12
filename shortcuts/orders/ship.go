package orders

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

var shipShortcut = common.Shortcut{
	Service: "orders",
	Command: "+ship",
	Use:     "+ship --order-id <id> --tracking <no> [--line-items <id:qty,...>]",
	Short:   "Create a fulfillment (ship an order)",
	Flags: []common.Flag{
		{Name: "order-id", Type: common.FlagString, Required: true, Description: "Order ID."},
		{Name: "tracking", Type: common.FlagString, Required: true, Description: "Tracking number."},
		{Name: "company", Type: common.FlagString, Description: "Carrier company name (e.g., DHL, UPS)."},
		{Name: "company-code", Type: common.FlagString, Description: "Carrier company code."},
		{Name: "line-items", Type: common.FlagString, Description: "Per-line qty as 'line-id:qty,...'; default: all fulfillable."},
		{Name: "notify", Type: common.FlagBool, Description: "Notify customer."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		orderID := in.Flags.GetString("order-id")
		tracking := in.Flags.GetString("tracking")
		company := in.Flags.GetString("company")
		companyCode := in.Flags.GetString("company-code")
		lineItemsArg := in.Flags.GetString("line-items")
		notify := in.Flags.GetBool("notify")

		getPlan := PlanGet(orderID)

		if in.DryRun {
			// Without fetching the order, build the body from --line-items only (no fulfillable check).
			fulfillment := map[string]any{
				"tracking_number": tracking,
			}
			if notify {
				fulfillment["send_email"] = true
			}
			cmdutil.AddString(fulfillment, "tracking_company", company)
			cmdutil.AddString(fulfillment, "tracking_company_code", companyCode)
			if lineItemsArg != "" {
				items, err := dryRunLineItemsFromArg(lineItemsArg)
				if err != nil {
					return common.ExecResult{}, err
				}
				fulfillment["line_items"] = items
			}
			postPlan := PlanCreateFulfillment(orderID, map[string]any{"fulfillment": fulfillment})
			return common.ExecResult{Plans: []common.PlannedRequest{getPlan, postPlan}}, nil
		}

		// Live: GET the order; after the envelope is stripped the response is {"order": {...}}.
		envelope, err := common.Send(ctx, in.Client, getPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		order, ok := envelope["order"].(map[string]any)
		if !ok {
			return common.ExecResult{}, output.ErrInternal("order response missing 'order' object")
		}
		fulfillment, err := buildShipBody(order, lineItemsArg, tracking, company, companyCode, notify)
		if err != nil {
			return common.ExecResult{}, err
		}
		resp, err := common.Send(ctx, in.Client, PlanCreateFulfillment(orderID, map[string]any{"fulfillment": fulfillment}))
		if err != nil {
			return common.ExecResult{}, err
		}
		return common.ExecResult{Body: resp}, nil
	},
}

// parseLineItemsArg parses "li-1:2,li-2:1" into {"li-1":2,"li-2":1}.
func parseLineItemsArg(arg string) (map[string]int, error) {
	out := map[string]int{}
	for _, pair := range strings.Split(arg, ",") {
		pair = strings.TrimSpace(pair)
		if pair == "" {
			continue
		}
		parts := strings.SplitN(pair, ":", 2)
		if len(parts) != 2 {
			return nil, output.ErrValidation("--line-items pair %q: expected 'line-id:qty'", pair)
		}
		id := strings.TrimSpace(parts[0])
		qty, err := strconv.Atoi(strings.TrimSpace(parts[1]))
		if err != nil {
			return nil, output.ErrValidation("--line-items pair %q: qty must be a positive integer", pair)
		}
		if qty <= 0 {
			return nil, output.ErrValidation("--line-items pair %q: qty must be > 0", pair)
		}
		out[id] = qty
	}
	return out, nil
}

// buildShipBody constructs the POST /fulfillments body from the order and the --line-items spec.
func buildShipBody(order map[string]any, lineItemsArg, tracking, company, companyCode string, notify bool) (map[string]any, error) {
	fulfillable, err := extractFulfillableQuantities(order)
	if err != nil {
		return nil, err
	}
	requested := fulfillable
	if lineItemsArg != "" {
		userSpec, perr := parseLineItemsArg(lineItemsArg)
		if perr != nil {
			return nil, perr
		}
		for id, qty := range userSpec {
			avail, ok := fulfillable[id]
			if !ok {
				return nil, output.ErrValidation("--line-items: line item %q not found on order", id)
			}
			if qty > avail {
				return nil, output.ErrValidation("--line-items: line item %q requested qty %d > fulfillable_quantity %d", id, qty, avail)
			}
		}
		requested = userSpec
	}
	items := make([]map[string]any, 0, len(requested))
	for id, qty := range requested {
		items = append(items, map[string]any{"id": id, "ship_quantity": qty})
	}
	body := map[string]any{
		"tracking_number": tracking,
		"line_items":      items,
	}
	if notify {
		body["send_email"] = true
	}
	cmdutil.AddString(body, "tracking_company", company)
	cmdutil.AddString(body, "tracking_company_code", companyCode)
	return body, nil
}

func extractFulfillableQuantities(order map[string]any) (map[string]int, error) {
	raw, ok := order["line_items"].([]any)
	if !ok {
		return nil, output.ErrInternal("order response missing line_items array")
	}
	out := map[string]int{}
	for _, item := range raw {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		// When fulfillable_quantity is absent, fall back to the line item's total quantity.
		var quantity int
		if fulfillableQuantity, ok := asInt(m["fulfillable_quantity"]); ok && fulfillableQuantity > 0 {
			quantity = fulfillableQuantity
		} else if totalQuantity, ok := asInt(m["quantity"]); ok && totalQuantity > 0 {
			quantity = totalQuantity
		} else {
			continue
		}
		out[id] = quantity
	}
	return out, nil
}

// asInt converts the numeric forms a decoded JSON value may hold (json.Number, float64, int, int64) to an int.
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

func dryRunLineItemsFromArg(arg string) ([]map[string]any, error) {
	parsed, err := parseLineItemsArg(arg)
	if err != nil {
		return nil, err
	}
	items := make([]map[string]any, 0, len(parsed))
	for id, qty := range parsed {
		items = append(items, map[string]any{"id": id, "ship_quantity": qty})
	}
	return items, nil
}
