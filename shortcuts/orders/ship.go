package orders

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var shipShortcut = common.Shortcut{
	Service: "orders",
	Command: "+ship",
	Use:     "+ship --order-id <id> --tracking <no> [--line-items <id:qty,...>]",
	Short:   "Create a fulfillment (ship an order)",
	Long:    "Ship an order by creating a fulfillment with a tracking number; run --dry-run first to preview the request.",
	Example: `  # Preview shipping the whole order
  shoplazza orders +ship --order-id 12345 --tracking SF123456789 --company SF --dry-run

  # Ship specific line items
  shoplazza orders +ship --order-id 12345 --tracking SF123456789 --line-items li-1:2,li-2:1`,
	Flags: []common.Flag{
		{Name: "order-id", Type: common.FlagString, Required: true, Description: "Order ID.", Picker: orderPicker},
		{Name: "tracking", Type: common.FlagString, Required: true, Description: "Tracking number."},
		{Name: "company", Type: common.FlagString, Description: "Carrier company name (e.g., DHL, UPS)."},
		{Name: "company-code", Type: common.FlagString, Description: "Carrier company code."},
		{Name: "tracking-url", Type: common.FlagString, Description: "Custom tracking URL."},
		{Name: "line-items", Type: common.FlagString, Description: "Per-line qty as 'line-id:qty,...'; default: every line's remaining quantity (ordered minus already shipped)."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		orderID := in.Flags.GetString("order-id")
		lineItemsArg := in.Flags.GetString("line-items")
		fulfillment := map[string]any{
			"tracking_number": in.Flags.GetString("tracking"),
		}
		cmdutil.AddString(fulfillment, "tracking_company", in.Flags.GetString("company"))
		cmdutil.AddString(fulfillment, "tracking_company_code", in.Flags.GetString("company-code"))
		cmdutil.AddString(fulfillment, "tracking_url", in.Flags.GetString("tracking-url"))

		getPlan := PlanGet(orderID)

		if in.DryRun {
			// Without fetching the order, build the body from --line-items only (no fulfillable check).
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
		items, err := shipLineItems(order, lineItemsArg)
		if err != nil {
			return common.ExecResult{}, err
		}
		fulfillment["line_items"] = items
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

// shipLineItems picks the line items to ship from the order and the --line-items spec.
func shipLineItems(order map[string]any, lineItemsArg string) ([]map[string]any, error) {
	fulfillable, err := extractFulfillableQuantities(order)
	if err != nil {
		return nil, err
	}
	requested := map[string]int{}
	for id, qty := range fulfillable {
		if qty > 0 {
			requested[id] = qty
		}
	}
	if lineItemsArg == "" && len(requested) == 0 {
		return nil, output.ErrValidation("nothing left to ship: every line item of this order has already been shipped")
	}
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
				return nil, output.ErrValidation("--line-items: line item %q requested qty %d, only %d left to ship", id, qty, avail)
			}
		}
		requested = userSpec
	}
	items := make([]map[string]any, 0, len(requested))
	for id, qty := range requested {
		items = append(items, map[string]any{"id": id, "ship_quantity": qty})
	}
	return items, nil
}

func extractFulfillableQuantities(order map[string]any) (map[string]int, error) {
	raw, ok := order["line_items"].([]any)
	if !ok {
		return nil, output.ErrInternal("order response missing line_items array")
	}
	shipped := map[string]int{}
	fulfillments, _ := order["fulfillments"].([]any)
	for _, f := range fulfillments {
		fm, ok := f.(map[string]any)
		if !ok || fm["status"] == "cancelled" {
			continue
		}
		items, _ := fm["line_items"].([]any)
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if n, ok := asInt(m["ship_quantity"]); ok {
				shipped[id] += n
			}
		}
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
		quantity, _ := asInt(m["quantity"])
		out[id] = max(quantity-shipped[id], 0)
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
