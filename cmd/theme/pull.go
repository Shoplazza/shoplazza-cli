package themecmd

import (
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

// pullMaxUnpackSize caps cumulative extracted bytes from a pulled zip (200 MB).
const pullMaxUnpackSize = 200 * 1024 * 1024

// newCmdPull builds `themes pull`: stream a remote theme zip to a tmp file, then
// unpack it into cwd (top-level dir stripped, path-traversal guarded). The tmp
// zip is preserved on failure so users can retry the unpack manually.
func newCmdPull(f *cmdutil.Factory) *cobra.Command {
	var themeID, environment string
	cmd := &cobra.Command{
		Use:   "pull",
		Short: "Download and unpack a remote theme into the current directory",
		Long:  "Download a theme and unpack it into the current directory. --theme-id is required (or comes from -e).",
		Example: `  # Download a theme and unpack it into the current directory
  shoplazza themes pull --theme-id 123456

  # Pull the theme configured for an environment
  shoplazza themes pull -e staging`,
		// Owns its (env-aware) auth — see push.go.
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			rs, err := resolveStore(ctx, f, cmd)
			if err != nil {
				return err
			}
			resolvedID, err := resolveThemeID(ctx, f, rs, themeID)
			if err != nil {
				return err
			}
			resolvedID, verr := theme.RequireThemeID(resolvedID)
			if verr != nil {
				return verr
			}
			start := time.Now()
			prog := output.NewProgress(cmd.ErrOrStderr())

			// Best-effort theme name for the header label; a detail-endpoint blip
			// must not abort the pull (the download gives the authoritative answer).
			var themeName string
			if resp, derr := rs.Client.DoRaw(ctx, client.RawRequest{Method: "GET", Path: themeBaseV202601 + "/" + resolvedID}); derr == nil {
				themeName = extractStringField(asMap(resp.Body), "name")
			}
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[pull] target theme: %s\n", themeLabel(themeName, resolvedID))

			dlStep := prog.Begin("[pull] downloading theme files")
			reader, err := rs.Client.SendStream(ctx, client.RawRequest{Method: "GET", Path: themeBaseV1 + "/" + resolvedID + "/download"})
			if err != nil {
				dlStep.Fail()
				return classifyPullDownloadErr(err, resolvedID)
			}
			defer func() { _ = reader.Close() }()
			outFile, err := createTempZip(resolvedID)
			if err != nil {
				dlStep.Fail()
				return theme.ErrLocalIO("create tmp zip", err)
			}
			tmpZip := outFile.Name()
			written, copyErr := io.Copy(outFile, reader)
			if cerr := outFile.Close(); copyErr == nil {
				copyErr = cerr
			}
			if copyErr != nil {
				dlStep.Fail()
				return theme.ErrLocalIO(fmt.Sprintf("write tmp zip (preserved at %s)", tmpZip), copyErr)
			}
			dlStep.Done()
			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "[pull] downloaded %d bytes to %s\n", written, tmpZip)

			exStep := prog.Begin("[pull] extracting to ./")
			cwd, gerr := os.Getwd()
			if gerr != nil {
				exStep.Fail()
				return theme.ErrLocalIO(fmt.Sprintf("getwd (tmp zip preserved at %s)", tmpZip), gerr)
			}
			if uerr := pack.Unpack(tmpZip, cwd, pack.UnpackOptions{StripTopDir: true, MaxTotalSize: pullMaxUnpackSize, PathTraversalCheck: true}); uerr != nil {
				exStep.Fail()
				switch {
				case errors.Is(uerr, pack.ErrUnsafeArchivePath):
					return theme.ErrValidation("%v (tmp zip preserved at %s)", uerr, tmpZip)
				case errors.Is(uerr, pack.ErrSizeLimit):
					return theme.ErrValidation("theme archive exceeds 200MB extracted size limit (tmp at %s)", tmpZip)
				default:
					return theme.ErrLocalIO(fmt.Sprintf("unpack theme zip (tmp preserved at %s)", tmpZip), uerr)
				}
			}
			_ = os.Remove(tmpZip)
			exStep.Done()

			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
				"theme_id":   resolvedID,
				"theme_name": themeName,
				"target":     "./",
				"elapsed_s":  roundedElapsed(start),
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVarP(&themeID, "theme-id", "t", "", "Theme ID (required unless -e provides it). Run 'shoplazza themes list' to discover")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment from shoplazza.theme.toml (store/profile/theme); see 'themes env list'")
	return cmd
}

// createTempZip makes a uniquely-named tmp file for the streamed download; the id
// is sanitized because CreateTemp rejects separators in the pattern.
func createTempZip(themeID string) (*os.File, error) {
	return os.CreateTemp("", "shoplazza-theme-"+theme.SanitizeFileComponent(themeID)+"-*.zip")
}

// extractStringField looks up a string key at the root, then data, then
// data.theme — the envelope shapes the detail endpoint has used.
func extractStringField(resp map[string]any, key string) string {
	for _, candidate := range []map[string]any{resp, mapChild(resp, "data"), mapChild(mapChild(resp, "data"), "theme")} {
		if candidate == nil {
			continue
		}
		if v, ok := candidate[key].(string); ok && v != "" {
			return v
		}
	}
	return ""
}

func mapChild(m map[string]any, k string) map[string]any {
	if m == nil {
		return nil
	}
	if v, ok := m[k].(map[string]any); ok {
		return v
	}
	return nil
}

// themeLabel renders "<name> (<id>)" when a distinct name is known, else "<id>".
func themeLabel(name, id string) string {
	if name == "" || name == id {
		return id
	}
	return fmt.Sprintf("%s (%s)", name, id)
}

// roundedElapsed returns elapsed seconds truncated to 1 decimal (stable JSON).
func roundedElapsed(start time.Time) float64 {
	return float64(int(time.Since(start).Seconds()*10)) / 10
}

// classifyPullDownloadErr maps a download stream failure to the right envelope.
func classifyPullDownloadErr(err error, themeID string) error {
	var he *client.HTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case http.StatusNotFound:
			return theme.ErrValidation("theme not found: %s (run `shoplazza themes list` to see available IDs)", themeID)
		case http.StatusUnauthorized, http.StatusForbidden:
			return theme.ErrAuthExpired(err)
		default:
			if he.StatusCode >= 500 {
				return fmt.Errorf("server error %d during download: %w", he.StatusCode, err)
			}
		}
	}
	return fmt.Errorf("download failed: %w", err)
}
