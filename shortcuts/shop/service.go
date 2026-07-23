package shop

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

const fileBase = common.APIPrefix + "/file"

func PlanFileUpload(sourceURLs []string, folder string) common.PlannedRequest {
	body := map[string]any{
		"original_source_list": sourceURLs,
	}
	if folder != "" {
		body["folder"] = folder
	}
	return common.PlannedRequest{Method: "POST", Path: fileBase, Body: body}
}

func PlanFileTask(taskID string) common.PlannedRequest {
	return common.PlannedRequest{Method: "GET", Path: fileBase + "/task/" + taskID}
}
