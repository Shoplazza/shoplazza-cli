package orders

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var refundShortcut = common.Shortcut{
	Service: "orders",
	Command: "+refund",
	Use:     "+refund --order-id <id> --amount <n> [--payment-line-id <id>]",
	Short:   "Refund an order",

	Destructive:       true,
	ConfirmPrompt:     "Refund this order? Money is returned to the buyer and cannot be undone.",
	ConfirmPhraseFlag: "order-id",
	Long: "Refund an amount against an order's payment line; run --dry-run first to preview the request.\n\n" +
		"An order paid through a custom payment channel (customize_payment) or the test channel (bogus) stays in progress after the refund; " +
		"complete it with 'orders refunds finish', passing the post_sale_id and refund_record_id from the response.",
	Example: `  # Preview a partial refund
  shoplazza orders +refund --order-id 12345 --amount 29.99 --dry-run

  # Refund and return inventory with a note
  shoplazza orders +refund --order-id 12345 --amount 29.99 --return-items --note 'damaged in transit'`,
	Flags: []common.Flag{
		{Name: "order-id", Type: common.FlagString, Required: true, Description: "Order ID.", Picker: orderPicker},
		{Name: "amount", Type: common.FlagString, Required: true, Description: "Refund amount (e.g., '29.99')."},
		{Name: "payment-line-id", Type: common.FlagString, Description: "Payment line ID (required when order has multiple payment_lines)."},
		{Name: "note", Type: common.FlagString, Description: "Optional note."},
		{Name: "return-items", Type: common.FlagBool, Description: "Also return inventory."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		orderID := in.Flags.GetString("order-id")
		amount := in.Flags.GetString("amount")
		paymentLineID := in.Flags.GetString("payment-line-id")
		note := in.Flags.GetString("note")
		returnItems := in.Flags.GetBool("return-items")

		if v, _ := strconv.ParseFloat(amount, 64); !refundAmountPattern.MatchString(amount) || v <= 0 {
			return common.ExecResult{}, output.ErrValidation("--amount must be a number greater than 0").WithParam("--amount")
		}

		getPlan := PlanGet(orderID)

		if in.DryRun {
			if paymentLineID == "" {
				paymentLineID = resolvedFromOrder
			}
			refund := buildRefundBody(paymentLineID, amount, note, false, nil)
			if returnItems {
				refund["refund_line_items"] = resolvedFromOrder
			}
			postPlan := PlanRefund(orderID, map[string]any{"refund": refund})
			return common.ExecResult{Plans: []common.PlannedRequest{getPlan, postPlan}}, nil
		}

		envelope, err := common.Send(ctx, in.Client, getPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		// After the transport envelope is stripped, /orders/{id} returns {"order": {...}}.
		order, ok := envelope["order"].(map[string]any)
		if !ok {
			return common.ExecResult{}, output.ErrInternal("order response missing 'order' object")
		}
		chosen, err := choosePaymentLine(order, paymentLineID)
		if err != nil {
			return common.ExecResult{}, err
		}
		var lineItems []any
		if returnItems {
			if rawItems, ok := order["line_items"].([]any); ok {
				lineItems = rawItems
			}
		}
		refund := buildRefundBody(chosen, amount, note, returnItems, lineItems)
		resp, err := common.Send(ctx, in.Client, PlanRefund(orderID, map[string]any{"refund": refund}))
		if err != nil {
			return common.ExecResult{}, err
		}
		return common.ExecResult{Body: resp}, nil
	},
}

const resolvedFromOrder = "<resolved-from-step-0>"

var refundAmountPattern = regexp.MustCompile(`^\d+(\.\d+)?$`)

// choosePaymentLine picks the payment line to refund, disambiguating when an order has multiple.
func choosePaymentLine(order map[string]any, userProvided string) (string, error) {
	raw, ok := order["payment_lines"].([]any)
	if !ok || len(raw) == 0 {
		return "", output.ErrInternal("order has no payment_lines")
	}
	ids := make([]string, 0, len(raw))
	idSet := map[string]bool{}
	for _, pl := range raw {
		m, ok := pl.(map[string]any)
		if !ok {
			continue
		}
		id, _ := m["id"].(string)
		if id == "" {
			continue
		}
		ids = append(ids, id)
		idSet[id] = true
	}
	if userProvided != "" {
		if !idSet[userProvided] {
			return "", output.ErrValidation("--payment-line-id %q is not a payment_line on order; available: %s", userProvided, strings.Join(ids, ", "))
		}
		return userProvided, nil
	}
	if len(ids) == 1 {
		return ids[0], nil
	}
	return "", output.ErrValidation("order has %d payment_lines; pick one with --payment-line-id (available: %s)", len(ids), strings.Join(ids, ", "))
}

func buildRefundBody(paymentLineID, amount, note string, returnItems bool, lineItems []any) map[string]any {
	body := map[string]any{
		"refund_payments": []map[string]any{
			{"payment_line_id": paymentLineID, "refund_price": amount},
		},
		"refund_total": amount,
	}
	cmdutil.AddString(body, "note", note)
	if returnItems && len(lineItems) > 0 {
		annotated := make([]map[string]any, 0, len(lineItems))
		for _, raw := range lineItems {
			m, ok := raw.(map[string]any)
			if !ok {
				continue
			}
			id, _ := m["id"].(string)
			if id == "" {
				continue
			}
			annotated = append(annotated, map[string]any{
				"line_item_id":     id,
				"return_inventory": true,
			})
		}
		body["refund_line_items"] = annotated
	}
	return body
}
