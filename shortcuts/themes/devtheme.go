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

// createDevTheme packs cwd and uploads it with an empty theme_id so the server
// allocates a new theme, then returns that id. Doubles as serve's initial push.
func createDevTheme(ctx context.Context, c *client.Client, prog *output.Progress, cwd, devName, version string) (string, error) {
	pkgStep := prog.Begin("[serve] packaging theme files")
	zipPath, err := pack.Pack(cwd, themeZipName(devName, version), pack.PackOptions{})
	if err != nil {
		pkgStep.Fail()
		return "", theme.ErrLocalIO("pack zip", err)
	}
	defer os.Remove(zipPath)
	pkgStep.Done()

	id, err := uploadZipResolveThemeID(ctx, c, prog, "[serve]", PlanShareUpload("", devName, version), zipPath, "")
	if err != nil {
		return "", err
	}
	if id == "" {
		return "", theme.ErrValidation(
			"server did not return a theme id for the development-theme upload; " +
				"re-run with --theme-id <id> to serve an existing theme")
	}
	return id, nil
}

// uploadZipResolveThemeID uploads zipPath via the multipart /themes/upload POST
// and resolves the resulting theme id, waiting on the upload task when the
// server only returns a task_id. Progress steps are printed under prefix.
// fallbackID is returned when neither an id nor a task comes back.
func uploadZipResolveThemeID(
	ctx context.Context,
	c *client.Client,
	prog *output.Progress,
	prefix string,
	uploadPlan common.PlannedRequest,
	zipPath, fallbackID string,
) (string, error) {
	uplStep := prog.Begin(fmt.Sprintf("%s uploading %s%s", prefix, filepath.Base(zipPath), zipSizeLabel(zipPath)))
	body, ct, err := multipartx.FileFormBody("file", zipPath, "application/zip", nil)
	if err != nil {
		uplStep.Fail()
		return "", theme.ErrLocalIO("build multipart", err)
	}
	// NoTimeout: large zips can exceed the client-wide timeout; ctx still cancels.
	resp, err := c.DoRaw(ctx, client.RawRequest{
		Method:    uploadPlan.Method,
		Path:      uploadPlan.Path,
		Params:    uploadPlan.Query,
		Data:      body,
		Headers:   map[string]string{"Content-Type": ct},
		NoTimeout: true,
	})
	if err != nil {
		uplStep.Fail()
		return "", classifyHTTPErr(err, fallbackID)
	}
	uplStep.Done()

	returnedThemeID := extractStringField(asMap(resp.Body), "theme_id")
	if returnedThemeID == "" {
		if taskID := extractTaskID(resp.Body); taskID != "" {
			fmt.Fprintf(os.Stderr, "%s upload task %s\n", prefix, taskID)
			waitStep := prog.Begin(prefix + " waiting for the server to process the theme")
			payload, perr := waitUploadTask(ctx, c, taskID)
			if perr != nil {
				waitStep.Fail()
				return "", perr
			}
			waitStep.Done()
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
