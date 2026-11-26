package shop

import (
	"context"
	"fmt"
	"net/url"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var uploadFileShortcut = common.Shortcut{
	Service: "shop",
	Command: "+upload-file",
	Use:     "+upload-file --source-url <url> [--source-url <url> ...]",
	Short:   "Submit a file upload task (takes public URLs, NOT local files)",
	Long:    "Submit a file-upload task that fetches public URLs into the store media library (not local files); run --dry-run first to preview.",
	Example: `  # Preview uploading one image URL
  shoplazza shop +upload-file --source-url https://cdn.example.com/a.jpg --dry-run

  # Upload several URLs
  shoplazza shop +upload-file --source-url https://cdn.example.com/a.jpg --source-url https://cdn.example.com/b.jpg`,
	Flags: []common.Flag{
		{Name: "source-url", Type: common.FlagStringArray, Required: true, Description: "Public http(s) URL for the API to fetch. Repeat the flag or join URLs with commas for several; a comma not followed by http:// or https:// stays part of the URL."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		urls := splitSourceURLs(in.Flags.GetStringArray("source-url"))
		if err := checkSourceURLs(urls); err != nil {
			return common.ExecResult{}, err
		}

		postPlan := PlanFileUpload(urls)

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

var nextSourceURL = regexp.MustCompile(`(?i),\s*https?://`)

// splitSourceURLs splits a value only where a comma starts another http(s) URL.
func splitSourceURLs(values []string) []string {
	var urls []string
	for _, v := range values {
		for loc := nextSourceURL.FindStringIndex(v); loc != nil; loc = nextSourceURL.FindStringIndex(v) {
			urls = append(urls, v[:loc[0]])
			v = strings.TrimLeft(v[loc[0]+1:], " \t")
		}
		urls = append(urls, v)
	}
	return urls
}

func checkSourceURLs(urls []string) error {
	var bad []string
	hint := "the API downloads each URL itself and cannot read local files; host the file and pass its public link"
	for _, raw := range urls {
		if u, err := url.Parse(raw); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
			bad = append(bad, fmt.Sprintf("%q", raw))
			if strings.HasPrefix(raw, "data:") {
				hint = `send data: URIs with 'shop files upload --data' ({"original_source_list":[...]}, inline, @file or - for stdin)`
			}
		}
	}
	if len(bad) == 0 {
		return nil
	}
	return output.ErrValidation("--source-url takes http(s) URLs, got %s", strings.Join(bad, ", ")).
		WithParam("--source-url").
		WithSubtype(output.SubtypeInvalidFlagValue).
		WithHint(hint)
}

func extractTaskID(resp map[string]any) string {
	v, _ := resp["task_id"].(string)
	return v
}
