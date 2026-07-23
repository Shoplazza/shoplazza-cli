package discounts

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

const discountsBase = common.APIPrefix + "/discounts"

// PlanList builds the GET request that lists discounts.
func PlanList(query map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: discountsBase, Query: query}
}

// PlanCreateAutomatic builds the POST request that creates an automatic discount.
func PlanCreateAutomatic(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: discountsBase + "/automatic", Body: body}
}

// PlanCreateNonAutomatic builds the POST request that creates a code (non-automatic) discount.
func PlanCreateNonAutomatic(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: discountsBase + "/non-automatic", Body: body}
}
