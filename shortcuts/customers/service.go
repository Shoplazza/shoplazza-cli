package customers

import "shoplazza-cli-v2/shortcuts/common"

const customersBase = common.APIPrefix + "/customers"

// PlanList builds a GET request to list customers with the given query.
func PlanList(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: customersBase, Query: query}
}

// PlanCreate builds a POST request to create a customer from the given body.
func PlanCreate(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: customersBase, Body: body}
}
