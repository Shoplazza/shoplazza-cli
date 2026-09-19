package themecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/asynctask"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/multipartx"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

const (
	themeBaseV202601 = "/openapi/2026-01/themes"
	themeBaseV1      = "/openapi/2020-07/themes"
)

// pushPollOpts controls the upload-task polling cadence (3s interval, 10-minute cap).
var pushPollOpts = asynctask.PollOptions{Interval: 3 * time.Second, MaxDuration: 10 * time.Minute}

// maxConsecutivePollErrors bounds consecutive transient poll errors before giving up.
var maxConsecutivePollErrors = 5

// newCmdPush builds `themes push`: package the cwd, upload it to a remote theme,
// and poll the upload task. Plain cobra (not a shortcut) so it owns its store
// client — which is what lets -e select the store/profile entirely locally.
func newCmdPush(f *cmdutil.Factory) *cobra.Command {
	var themeID, taskID, environment string
	cmd := &cobra.Command{
		Use:   "push",
		Short: "Package and upload the current theme to a remote theme",
		Long:  "Package the current directory and upload it to a theme, then wait for the server to process it. --theme-id is required (or comes from -e); it OVERWRITES the theme's files on the server.",
		Example: `  # Package the current directory and upload it to a theme
  shoplazza themes push --theme-id 123456

  # Push to the theme configured for an environment
  shoplazza themes push -e staging`,
		// Owns its auth: resolveStore mints the (env-aware) store token, so the
		// shared `themes` module gate (RequireAuth, which is not -e aware) must
		// skip this command rather than resolve the wrong/absent profile first.
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			// resolveStore owns auth: it resolves the (env-aware) profile and mints
			// its store token, so an unauthenticated target surfaces as a precise,
			// structured error here — no separate coarse login gate needed.
			rs, err := resolveStore(ctx, f, cmd)
			if err != nil {
				return err
			}
			// theme-id: explicit flag > environment's theme= > interactive pick.
			resolvedID, err := resolveThemeID(ctx, f, rs, themeID)
			if err != nil {
				return err
			}
			resolvedID, verr := theme.RequireThemeID(resolvedID)
			if verr != nil {
				return verr
			}

			// Human-only confirmation: push overwrites the theme's files on the store.
			if err := cmdutil.ConfirmDestructive(f,
				"Push this package to theme "+resolvedID+" on "+rs.Domain+"? It overwrites the theme's files on the server."); err != nil {
				return err
			}

			prog := output.NewProgress(cmd.ErrOrStderr())
			payload, err := pushTheme(ctx, prog, cmd.ErrOrStderr(), rs.Client, resolvedID, taskID)
			if err != nil {
				return err
			}
			// Record the resolved target into the default environment, same rules as
			// pull (create if absent; confirm before overwriting an existing default;
			// skipped under -e). A one-off push declines that overwrite prompt.
			cwd, _ := os.Getwd()
			maybeWriteThemeEnv(cmd, f, cwd, rs, resolvedID, "push")

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "✓ pushed to theme %s on %s\n", resolvedID, rs.Domain)
			return output.PrintAPISuccess(cmd.OutOrStdout(),
				map[string]any{"theme_id": resolvedID, "task": payload}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVarP(&themeID, "theme-id", "t", "", "Theme ID (required unless -e provides it). Run 'shoplazza themes list' to discover")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Resume waiting for an earlier upload task instead of uploading again (task_id from a timeout error)")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment from shoplazza.theme.toml (store/profile/theme); see 'themes env list'")
	return cmd
}

// pushTheme runs the detail → pack → upload → poll pipeline against themeID and
// returns the finished task payload. Shared by `themes push` and serve's initial
// push so both behave identically. taskID != "" resumes an earlier upload task
// instead of re-uploading.
func pushTheme(ctx context.Context, prog *output.Progress, errW io.Writer, c *client.Client, themeID, taskID string) (map[string]any, error) {
	// detail GET confirms the theme exists — no point packaging a zip that can't
	// be delivered.
	if _, derr := c.DoRaw(ctx, client.RawRequest{Method: "GET", Path: themeBaseV202601 + "/" + themeID}); derr != nil {
		return nil, classifyHTTPErr(derr, themeID)
	}
	if taskID == "" {
		cwd, err := os.Getwd()
		if err != nil {
			return nil, theme.ErrLocalIO("getwd", err)
		}
		name, version, rerr := theme.ReadInfo(cwd)
		if rerr != nil {
			return nil, rerr
		}
		if taskID, err = packAndUpload(ctx, c, prog, cwd, themeID, name, version); err != nil {
			return nil, err
		}
		_, _ = fmt.Fprintf(errW, "[push] upload task %s\n", taskID)
	} else {
		_, _ = fmt.Fprintf(errW, "[push] resuming upload task %s\n", taskID)
	}
	waitStep := prog.Begin("[push] waiting for the server to process the theme")
	payload, err := waitUploadTask(ctx, c, taskID)
	if err != nil {
		waitStep.Fail()
		return nil, err
	}
	waitStep.Done()
	decodeTaskJSONFields(payload)
	return payload, nil
}

// resolveThemeID fills the theme id: an explicit flag wins; else the selected
// environment's theme=; else a fuzzy pick from the store's themes (interactive
// only). Non-interactive with nothing to go on returns "" for RequireThemeID to
// reject with the structured "theme id is required" error.
func resolveThemeID(ctx context.Context, f *cmdutil.Factory, rs resolvedStore, flagID string) (string, error) {
	if flagID != "" {
		return flagID, nil
	}
	if rs.Env.Theme != "" {
		return rs.Env.Theme, nil
	}
	if !cmdutil.Interactive(f) {
		return "", nil
	}
	opts, err := themeOptions(ctx, rs.Client)
	if err != nil || len(opts) == 0 {
		return "", nil // lookup failed/empty → let RequireThemeID prompt the flag error
	}
	return interact.SelectFiltered("Theme to push to", opts)
}

// themeOptions lists the store's themes for the picker (value = id, label = name·role).
func themeOptions(ctx context.Context, c *client.Client) ([]interact.Option, error) {
	resp, err := c.DoRaw(ctx, client.RawRequest{Method: "GET", Path: themeBaseV202601})
	if err != nil {
		return nil, err
	}
	arr, _ := digThemes(resp.Body)
	opts := make([]interact.Option, 0, len(arr))
	for _, t := range arr {
		id := asString(t["id"])
		if id == "" {
			continue
		}
		label := asString(t["name"])
		if label == "" {
			label = asString(t["title"])
		}
		if label == "" {
			label = id
		}
		if role := asString(t["theme_type"]); role != "" {
			label += " · " + role
		}
		opts = append(opts, interact.Option{Label: label, Value: id})
	}
	return opts, nil
}

// digThemes finds the themes array in a list response ({themes:[...]},
// {data:{themes:[...]}}, {data:[...]} or a bare array).
func digThemes(body any) ([]map[string]any, bool) {
	toMaps := func(a []any) []map[string]any {
		out := make([]map[string]any, 0, len(a))
		for _, it := range a {
			if m, ok := it.(map[string]any); ok {
				out = append(out, m)
			}
		}
		return out
	}
	if a, ok := body.([]any); ok {
		return toMaps(a), true
	}
	m, ok := body.(map[string]any)
	if !ok {
		return nil, false
	}
	if a, ok := m["themes"].([]any); ok {
		return toMaps(a), true
	}
	switch d := m["data"].(type) {
	case []any:
		return toMaps(d), true
	case map[string]any:
		if a, ok := d["themes"].([]any); ok {
			return toMaps(a), true
		}
	}
	return nil, false
}

// packAndUpload zips cwd, uploads it to themeID, and returns the upload task id.
func packAndUpload(ctx context.Context, c *client.Client, prog *output.Progress, cwd, themeID, name, version string) (string, error) {
	pkgStep := prog.Begin("[push] packaging theme files")
	zipPath, err := pack.Pack(cwd, theme.ZipName(name, version), pack.PackOptions{})
	if err != nil {
		pkgStep.Fail()
		return "", theme.ErrLocalIO("pack zip", err)
	}
	defer func() { _ = os.Remove(zipPath) }()
	pkgStep.Done()

	uplStep := prog.Begin("[push] uploading " + theme.ZipName(name, version))
	body, ct, err := multipartx.FileFormBody("file", zipPath, "application/zip", nil)
	if err != nil {
		uplStep.Fail()
		return "", theme.ErrLocalIO("build multipart", err)
	}
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
		return "", classifyHTTPErr(err, themeID)
	}
	taskID := extractTaskID(resp.Body)
	if taskID == "" {
		uplStep.Fail()
		return "", theme.ErrLocalIO("upload response missing task_id", fmt.Errorf("response body: %v", resp.Body))
	}
	uplStep.Done()
	return taskID, nil
}
