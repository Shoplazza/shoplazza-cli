package themes

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// pullMaxUnpackSize caps the cumulative extracted bytes from a pulled zip.
// 200 MB matches the default in internal/theme/pack, declared explicitly here
// so the policy is visible at the call site.
const pullMaxUnpackSize = 200 * 1024 * 1024

// pullShortcut is the `themes pull` workflow: streams a remote theme zip to a
// tmp file, then unpacks it into cwd with the top-level dir stripped. The tmp
// zip is preserved on failure so users can retry the unpack manually.
//
// PlanDetail uses the v2 spec path (best-effort, name only used for the
// progress label). PlanDownload uses the v1 download path because no v2
// equivalent exists in the spec (see service.go header comment).
var pullShortcut = common.Shortcut{
	Service: "themes",
	Command: "pull",
	Use:     "pull --theme-id <id>",
	Short:   "Download and unpack a remote theme into the current directory",
	Flags: []common.Flag{
		{
			Name:        "theme-id",
			Short:       "t",
			Type:        common.FlagString,
			Required:    true,
			Description: "Theme ID (required). Run `shoplazza themes list` to discover.",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		themeID, err := theme.RequireThemeID(in.Flags.GetString("theme-id"))
		if err != nil {
			return common.ExecResult{}, err
		}
		start := time.Now()

		detail := PlanDetail(themeID)
		download := PlanDownload(themeID)

		if in.DryRun {
			// Dry-run: emit BOTH planned requests so users see the exact
			// URLs that will fire. No filesystem side effects.
			return common.ExecResult{Plans: []common.PlannedRequest{detail, download}}, nil
		}

		prog := output.NewProgress(os.Stderr)

		// Step 1 (best-effort): theme name for the header label. Errors are
		// swallowed — a 404/network blip on the detail endpoint must NOT abort
		// the pull; the download step gives the authoritative "exists?" answer.
		var themeName string
		if body, derr := common.Send(ctx, in.Client, detail); derr == nil {
			themeName = extractStringField(body, "name")
		}
		// Header (no timer): identify the target. The "(id)" suffix appears only
		// when a distinct name is known, so an empty name never duplicates the
		// id as "<id> (<id>)".
		fmt.Fprintf(os.Stderr, "[pull] target theme: %s\n", themeLabel(themeName, themeID))

		// Step 2: stream the download to a uniquely-named tmp zip, kept on
		// failure so users can retry the unpack manually. Errors raised before
		// createTempZip succeeds must not mention the tmp path (no file exists yet).
		dlStep := prog.Begin("[pull] downloading theme files")
		reader, err := common.SendStream(ctx, in.Client, download)
		if err != nil {
			dlStep.Fail()
			return common.ExecResult{}, classifyPullDownloadErr(err, themeID)
		}
		defer reader.Close()
		out, err := createTempZip(themeID)
		if err != nil {
			dlStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO("create tmp zip", err)
		}
		tmpZip := out.Name()
		written, copyErr := io.Copy(out, reader)
		// A failed Close means buffered bytes never hit the disk — the zip
		// is corrupt even when Copy reported success, so it must fail the
		// download too (first error wins).
		if cerr := out.Close(); copyErr == nil {
			copyErr = cerr
		}
		if copyErr != nil {
			// Tmp zip is partial — preserve it anyway. The path is in the
			// message so triage can decide whether to keep or `rm` it.
			dlStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO(
				fmt.Sprintf("write tmp zip (preserved at %s)", tmpZip), copyErr)
		}
		dlStep.Done()
		fmt.Fprintf(os.Stderr, "[pull] downloaded %d bytes to %s\n", written, tmpZip)

		// Step 3: unpack into cwd with StripTopDir + path-traversal guard.
		exStep := prog.Begin("[pull] extracting to ./")
		cwd, gerr := os.Getwd()
		if gerr != nil {
			exStep.Fail()
			return common.ExecResult{}, theme.ErrLocalIO(
				fmt.Sprintf("getwd (tmp zip preserved at %s)", tmpZip), gerr)
		}
		err = pack.Unpack(tmpZip, cwd, pack.UnpackOptions{
			StripTopDir:        true,
			MaxTotalSize:       pullMaxUnpackSize,
			PathTraversalCheck: true,
		})
		if err != nil {
			// Tmp zip is preserved for forensics in all failure modes here.
			// Typed sentinels (errors.Is) pick the class; the wrapped message
			// names the OFFENDING ENTRY for traversal, not the tmp zip.
			exStep.Fail()
			switch {
			case errors.Is(err, pack.ErrUnsafeArchivePath):
				return common.ExecResult{}, theme.ErrValidation(
					"%v (tmp zip preserved at %s)", err, tmpZip)
			case errors.Is(err, pack.ErrSizeLimit):
				return common.ExecResult{}, theme.ErrValidation(
					"theme archive exceeds 200MB extracted size limit (tmp at %s)", tmpZip)
			default:
				// Local extraction failure (corrupt zip, permission denied,
				// disk full) — internal-class; "check network" here was a
				// verified mislead for e.g. permission errors.
				return common.ExecResult{}, theme.ErrLocalIO(
					fmt.Sprintf("unpack theme zip (tmp preserved at %s)", tmpZip), err)
			}
		}
		// Success — clean up the tmp zip. Ignore the remove error: at
		// worst we leak a dotfile-sized stale zip the OS will clean.
		_ = os.Remove(tmpZip)
		exStep.Done()

		return common.ExecResult{Body: map[string]any{
			"theme_id":   themeID,
			"theme_name": themeName,
			"target":     "./",
			"elapsed_s":  roundedElapsed(start),
		}}, nil
	},
}

// createTempZip creates a uniquely-named tmp file for the streamed download
// (os.CreateTemp is collision-proof via O_EXCL). The id component is sanitized
// because CreateTemp rejects separators in the pattern rather than escaping
// them — and so the name stays safe if a caller bypasses flag-level validation.
func createTempZip(themeID string) (*os.File, error) {
	return os.CreateTemp("", "shoplazza-theme-"+sanitizeFileComponent(themeID)+"-*.zip")
}

// extractStringField looks up `key` (a string field) at the root of resp,
// then under resp["data"], then under resp["data"]["theme"] — the three
// envelope shapes the v2 detail endpoint has used. Returns "" on any miss.
func extractStringField(resp map[string]any, key string) string {
	for _, candidate := range []map[string]any{
		resp,
		mapField(resp, "data"),
		mapField(mapField(resp, "data"), "theme"),
	} {
		if candidate == nil {
			continue
		}
		if v, ok := candidate[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

// mapField returns m[k] as a map[string]any, or nil. Nil-safe input.
func mapField(m map[string]any, k string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[k].(map[string]any); ok {
		return v
	}
	return nil
}

// themeLabel renders the theme for the progress header: "<name> (<id>)" when a
// distinct name is known, otherwise just "<id>".
func themeLabel(name, id string) string {
	if name == "" || name == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// roundedElapsed returns elapsed seconds truncated to 1 decimal as a
// float64 — used in the success body so JSON output is stable across runs.
func roundedElapsed(start time.Time) float64 {
	secs := time.Since(start).Seconds()
	return float64(int(secs*10)) / 10
}

// classifyPullDownloadErr maps a SendStream failure to the right v2 envelope:
//
//	404                    → validation (theme not found, hint to list)
//	401 / 403              → auth (refresh credentials)
//	5xx                    → raw error (engine maps to api)
//	non-HTTP (dial/timeout) → raw error (engine maps to network)
//
// No tmp-zip path appears here: SendStream fails before the tmp file is
// created, so mentioning it would point users at a nonexistent file.
func classifyPullDownloadErr(err error, themeID string) error {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		switch httpErr.StatusCode {
		case http.StatusNotFound:
			return theme.ErrValidation(
				"theme not found: %s (run `shoplazza themes list` to see available IDs)", themeID)
		case http.StatusUnauthorized, http.StatusForbidden:
			return theme.ErrAuthExpired(err)
		default:
			if httpErr.StatusCode >= 500 {
				return fmt.Errorf("server error %d during download: %w",
					httpErr.StatusCode, err)
			}
		}
	}
	return fmt.Errorf("download failed: %w", err)
}
