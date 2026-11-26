package shop

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

const fileBase = common.APIPrefix + "/file"

func PlanFileUpload(sourceURLs []string) common.PlannedRequest {
	return common.PlannedRequest{Method: "POST", Path: fileBase, Body: map[string]any{"original_source_list": sourceURLs}}
}

func PlanFileTask(taskID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: fileBase + "/task/" + taskID}
}
