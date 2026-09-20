package themecmd

import (
	"context"
	"encoding/json"
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
)

// devThemeName prefixes the local theme name so the sandbox is identifiable in
// the admin theme list.
func devThemeName(localName string) string { return "Development - " + localName }

// createDevTheme packs cwd and uploads it with an EMPTY theme_id so the server
// allocates a new theme, returning that id. Doubles as serve's initial push.
func createDevTheme(ctx context.Context, c *client.Client, prog *output.Progress, cwd, devName, version string) (string, error) {
	pkgStep := prog.Begin("[serve] packaging theme files")
	zipPath, err := pack.Pack(cwd, theme.ZipName(devName, version), pack.PackOptions{})
	if err != nil {
		pkgStep.Fail()
		return "", theme.ErrLocalIO("pack zip", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	pkgStep.Done()

	id, err := uploadZipResolveThemeID(ctx, c, prog, "[serve]", "", devName, version, zipPath, "")
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

// uploadZipResolveThemeID POSTs the multipart /themes/upload for (themeID, name,
// version) — themeID "" creates a new theme — and resolves the resulting theme
// id, waiting on the upload task when the server returns only a task_id.
// fallbackID is returned when neither an id nor a task comes back.
func uploadZipResolveThemeID(ctx context.Context, c *client.Client, prog *output.Progress, prefix, themeID, name, version, zipPath, fallbackID string) (string, error) {
	uplStep := prog.Begin(fmt.Sprintf("%s uploading %s%s", prefix, filepath.Base(zipPath), zipSizeLabel(zipPath)))
	body, ct, err := multipartx.FileFormBody("file", zipPath, "application/zip", nil)
	if err != nil {
		uplStep.Fail()
		return "", theme.ErrLocalIO("build multipart", err)
	}
	// NoTimeout: large zips can exceed the client-wide timeout; ctx still cancels.
	resp, err := c.DoRaw(ctx, client.RawRequest{
		Method:    "POST",
		Path:      themeBaseV1 + "/upload",
		Params:    map[string]any{"name": name, "version": version, "merchant_theme_id": "", "theme_id": themeID},
		Data:      body,
		Headers:   map[string]string{"Content-Type": ct},
		NoTimeout: true,
	})
	if err != nil {
		uplStep.Fail()
		return "", classifyHTTPErr(err, fallbackID)
	}
	uplStep.Done()

	id := extractStringField(asMap(resp.Body), "theme_id")
	if id == "" {
		if taskID := extractTaskID(resp.Body); taskID != "" {
			_, _ = fmt.Fprintf(os.Stderr, "%s upload task %s\n", prefix, taskID)
			waitStep := prog.Begin(prefix + " waiting for the server to process the theme")
			payload, perr := waitUploadTask(ctx, c, taskID)
			if perr != nil {
				waitStep.Fail()
				return "", perr
			}
			waitStep.Done()
			id = themeIDFromTask(payload)
		}
	}
	if id == "" {
		id = fallbackID
	}
	return id, nil
}

// adoptDevTheme saves id as the directory's development theme and renames it
// (best-effort — a rename failure leaves the uploaded name and is not fatal).
func adoptDevTheme(ctx context.Context, c *client.Client, prog *output.Progress, cwd, storeKey, id, devName string) error {
	if err := devstate.Save(cwd, storeKey, id); err != nil {
		return theme.ErrLocalIO("write .shoplazza/theme-state.json", err)
	}
	_, _ = fmt.Fprintf(os.Stderr, "[serve] development theme %s created (id saved to %s)\n",
		id, filepath.ToSlash(filepath.Join(".shoplazza", "theme-state.json")))
	step := prog.Begin(fmt.Sprintf("[serve] naming development theme %q", devName))
	if _, err := c.DoRaw(ctx, client.RawRequest{Method: "PATCH", Path: themeBaseV202601 + "/" + id + "/name", Data: map[string]any{"name": devName}}); err != nil {
		step.Fail()
		_, _ = fmt.Fprintf(os.Stderr, "[serve] warning: development theme keeps its uploaded name: %v\n", err)
		return nil
	}
	step.Done()
	return nil
}

// themeIDFromTask pulls a theme id out of a finished upload-task payload: a
// top-level theme_id, else one embedded in the JSON-encoded "info" string.
func themeIDFromTask(task map[string]any) string {
	if id := getString(task, "theme_id"); id != "" {
		return id
	}
	if info := getString(task, "info"); info != "" {
		var parsed struct {
			ThemeID string `json:"theme_id"`
		}
		if err := json.Unmarshal([]byte(info), &parsed); err == nil && parsed.ThemeID != "" {
			return parsed.ThemeID
		}
	}
	return ""
}

// isHTTPNotFound reports whether err is an HTTP 404 from the client layer.
func isHTTPNotFound(err error) bool {
	var he *client.HTTPError
	return errors.As(err, &he) && he.StatusCode == http.StatusNotFound
}

// zipSizeLabel returns " (<n> bytes)" for zipPath, or "" when it cannot be stat'ed.
func zipSizeLabel(zipPath string) string {
	info, err := os.Stat(zipPath)
	if err != nil {
		return ""
	}
	return fmt.Sprintf(" (%d bytes)", info.Size())
}
