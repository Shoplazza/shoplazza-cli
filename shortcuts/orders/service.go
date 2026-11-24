package orders

import (
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const ordersBase = common.APIPrefix + "/orders"

var (
	orderStatuses       = []string{"opened", "placed", "finished", "cancelled"}
	financialStatuses   = []string{"waiting", "paying", "authorized", "partially_paid", "paid", "cancelled", "failed", "refunding", "refund_failed", "refunded", "partially_refunded"}
	fulfillmentStatuses = []string{"initialled", "waiting", "partially_shipped", "shipped", "partially_finished", "finished", "cancelled", "returning", "partially_returned", "returned"}
)

// statusFlags are the order-state filters shared by +search and +count.
func statusFlags() []common.Flag {
	flag := func(name, what string, values []string) common.Flag {
		return common.Flag{Name: name, Type: common.FlagStringSlice, Enum: values,
			Description: "Filter by " + what + ": " + strings.Join(values, "|") +
				". Several values (comma-separated or repeated) match orders in any of them."}
	}
	return []common.Flag{
		flag("status", "order status", orderStatuses),
		flag("financial-status", "payment status", financialStatuses),
		flag("fulfillment-status", "fulfillment status", fulfillmentStatuses),
	}
}

// addStatusFilters sends each status flag as an array query parameter
// (?status=a&status=b).
func addStatusFilters(q map[string]any, in common.PlanInput) {
	cmdutil.AddSlice(q, "status", in.Flags.GetStringSlice("status"))
	cmdutil.AddSlice(q, "financial_status", in.Flags.GetStringSlice("financial-status"))
	cmdutil.AddSlice(q, "fulfillment_status", in.Flags.GetStringSlice("fulfillment-status"))
}

// PlanList builds a request to list orders.
func PlanList(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: ordersBase, Query: query}
}

// PlanGet builds a request to fetch a single order.
func PlanGet(orderID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: ordersBase + "/" + orderID}
}

// PlanCount builds a request to count orders.
func PlanCount(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: ordersBase + "/count", Query: query}
}

// PlanCreateFulfillment builds a request to create a fulfillment on an order.
func PlanCreateFulfillment(orderID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: ordersBase + "/" + orderID + "/fulfillments", Body: body}
}

// PlanUpdateFulfillment builds a request to update an existing fulfillment.
func PlanUpdateFulfillment(orderID, fulfillmentID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "PUT", Path: ordersBase + "/" + orderID + "/fulfillments/" + fulfillmentID, Body: body}
}

// PlanRefund builds a request to refund an order.
func PlanRefund(orderID string, body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: ordersBase + "/" + orderID + "/refund", Body: body}
}
