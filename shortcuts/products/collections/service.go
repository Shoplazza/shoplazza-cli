package collections

import "shoplazza-cli-v2/shortcuts/common"

const (
	collectionsBase = common.APIPrefix + "/collections"
	collectsBatch   = common.APIPrefix + "/collects/batch"
)

func PlanCreate(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: collectionsBase, Body: body}
}

func PlanBatchAssociate(body map[string]any) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: collectsBatch, Body: body}
}
