package shop

import (
	"context"

	"shoplazza-cli-v2/shortcuts/common"
)

var uploadFileShortcut = common.Shortcut{
	Service: "shop",
	Command: "+upload-file",
	Use:     "+upload-file --source-url <url> [--source-url <url> ...]",
	Short:   "Submit a file upload task (takes public URLs, NOT local files)",
	Flags: []common.Flag{
		{Name: "source-url", Type: common.FlagStringSlice, Required: true, Description: "Public URL(s) for the API to fetch. Can repeat."},
		{Name: "folder", Type: common.FlagString, Default: "all_upload", Description: "Target folder.",
			Completions: []string{"all_upload", "product"}},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		urls := in.Flags.GetStringSlice("source-url")
		folder := in.Flags.GetString("folder")

		postPlan := PlanFileUpload(urls, folder)

		if in.DryRun {
			// Dry-run shows both POST and GET shape (with placeholder task_id).
			getPlan := PlanFileTask("<resolved-from-step-0>")
			return common.ExecResult{Plans: []common.PlannedRequest{postPlan, getPlan}}, nil
		}

		postResp, err := common.Send(ctx, in.Client, postPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		taskID := extractTaskID(postResp)
		if taskID == "" {
			// Nothing more we can do — return what we have.
			return common.ExecResult{Body: postResp}, nil
		}
		// Single GET, NO polling.
		taskResp, err := common.Send(ctx, in.Client, PlanFileTask(taskID))
		if err != nil {
			return common.ExecResult{}, err
		}
		if taskResp == nil {
			taskResp = map[string]any{}
		}
		if _, present := taskResp["task_id"]; !present {
			taskResp["task_id"] = taskID
		}
		return common.ExecResult{Body: taskResp}, nil
	},
}

func extractTaskID(resp map[string]any) string {
	v, _ := resp["task_id"].(string)
	return v
}
