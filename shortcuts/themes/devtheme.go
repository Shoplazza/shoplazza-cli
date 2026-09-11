package themes

import (
	"context"
	"errors"
	"fmt"
	"net/http"
	"os"
	"path/filepath"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/devstate"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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
			payload, perr := waitUploadTask(ctx, c, taskID)
			if perr != nil {
				return "", perr
			}
			returnedThemeID = themeIDFromTask(payload)
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

// adoptDevTheme saves id as the directory's development theme and renames it (best-effort).
func adoptDevTheme(ctx context.Context, c *client.Client, prog *output.Progress, cwd, storeKey, id, devName string) error {
	if err := devstate.Save(cwd, storeKey, id); err != nil {
		return theme.ErrLocalIO("write .shoplazza/theme-state.json", err)
	}
	fmt.Fprintf(os.Stderr, "[serve] development theme %s created (id saved to %s)\n",
		id, filepath.ToSlash(filepath.Join(".shoplazza", "theme-state.json")))
	step := prog.Begin(fmt.Sprintf("[serve] naming development theme %q", devName))
	if _, err := common.Send(ctx, c, PlanRename(id, devName)); err != nil {
		step.Fail()
		fmt.Fprintf(os.Stderr, "[serve] warning: development theme keeps its uploaded name: %v\n", err)
		return nil
	}
	step.Done()
	return nil
}
