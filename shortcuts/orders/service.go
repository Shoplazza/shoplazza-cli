package orders

import "github.com/Shoplazza/shoplazza-cli/shortcuts/common"

const ordersBase = common.APIPrefix + "/orders"

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
