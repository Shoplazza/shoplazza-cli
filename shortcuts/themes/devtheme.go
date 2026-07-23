package themes

import (
	"context"
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/asynctask"
	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

// devThemeName derives the development theme's display name from the local
// theme_info name, prefixing it so the sandbox is identifiable in the admin
// theme list.
func devThemeName(localName string) string {
	return "Development - " + localName
}

// createDevTheme packs cwd and uploads it with an EMPTY theme_id query, which
// makes the upload endpoint allocate a brand-new theme, and returns its id.
// The upload doubles as serve's initial full push, so the caller can skip
// pushShortcut afterward.
func createDevTheme(ctx context.Context, c *client.Client, cwd, devName, version string) (string, error) {
	zipPath, err := pack.Pack(cwd, themeZipName(devName, version), pack.PackOptions{})
	if err != nil {
		return "", theme.ErrLocalIO("pack zip", err)
	}
	defer os.Remove(zipPath)

	id, err := uploadZipResolveThemeID(ctx, c, PlanShareUpload("", devName, version), zipPath, "")
	if err != nil {
		return "", err
	}
	if id == "" {
		// Without an id there is nothing to watch against — treat as a server
		// contract violation.
		return "", theme.ErrValidation(
			"server did not return a theme id for the development-theme upload; " +
				"re-run with --theme-id <id> to serve an existing theme")
	}
	return id, nil
}

// uploadZipResolveThemeID performs the multipart /themes/upload POST and
// resolves the resulting theme id. The endpoint may echo theme_id
// synchronously or return only a task_id for an async job; in the async case
// the upload task is polled and the id read from its info payload.
//
// fallbackID is returned when the server neither echoes an id nor runs an
// async task; pass "" when a missing id must be surfaced to the caller.
// Shared by `themes share` and serve's development-theme creation so the
// transport, task-polling, and id-extraction semantics stay identical.
func uploadZipResolveThemeID(
	ctx context.Context,
	c *client.Client,
	uploadPlan common.PlannedRequest,
	zipPath, fallbackID string,
) (string, error) {
	// Multipart goes through client.DoRaw (not common.Send) because
	// PlannedRequest has no Headers field and multipart transport requires
	// a per-request Content-Type with the runtime boundary.
	body, ct, err := multipartx.FileFormBody("file", zipPath, "application/zip", nil)
	if err != nil {
		return "", theme.ErrLocalIO("build multipart", err)
	}
	// NoTimeout: theme zips can exceed the client-wide 30s timeout on slow
	// uplinks; ctx (signal-cancelable) still aborts on Ctrl-C.
	resp, err := c.DoRaw(ctx, client.RawRequest{
		Method:    uploadPlan.Method,
		Path:      uploadPlan.Path,
		Params:    uploadPlan.Query,
		Data:      body,
		Headers:   map[string]string{"Content-Type": ct},
		NoTimeout: true,
	})
	if err != nil {
		return "", classifyHTTPErr(err, fallbackID)
	}

	returnedThemeID := extractStringField(asMap(resp.Body), "theme_id")
	if returnedThemeID == "" {
		if taskID := extractTaskID(resp.Body); taskID != "" {
			waitStart := time.Now()
			st, perr := asynctask.Poll(ctx, func(ctx context.Context) (asynctask.Status, error) {
				tr, terr := common.Send(ctx, c, PlanTaskDetail(taskID))
				if terr != nil {
					return asynctask.Status{}, terr
				}
				task := extractTaskPayload(tr)
				code := taskStatusCode(task["status"])
				return asynctask.Status{
					Done:    code != 0,
					Success: code == 1,
					Message: getString(task, "message"),
					Payload: task,
				}, nil
			}, pushPollOpts)
			if perr != nil {
				if errors.Is(perr, asynctask.ErrTimeout) {
					return "", theme.ErrTaskTimeout(
						time.Since(waitStart), pushPollOpts.MaxDuration, st.Payload)
				}
				return "", perr
			}
			if !st.Success {
				return "", theme.ErrTaskBusinessFailure(st.Payload)
			}
			returnedThemeID = themeIDFromTask(st.Payload)
		}
	}
	if returnedThemeID == "" {
		returnedThemeID = fallbackID
	}
	return returnedThemeID, nil
}

// isHTTPNotFound reports whether err is an HTTP 404 from the client layer.
func isHTTPNotFound(err error) bool {
	var httpErr *client.HTTPError
	return errors.As(err, &httpErr) && httpErr.StatusCode == http.StatusNotFound
}
