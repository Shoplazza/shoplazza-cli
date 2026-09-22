package themecmd

import (
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/env"
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

			// Bridge pull -> environments: record what we just pulled so the theme
			// dir is immediately -e aware (best-effort; never fails the pull).
			maybeWriteThemeEnv(cmd, f, cwd, rs, resolvedID, "pull")

			_, _ = fmt.Fprintf(cmd.ErrOrStderr(), "✓ pulled %s into ./\n", themeLabel(themeName, resolvedID))
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

// envWriteAction reports what recordThemeEnvironment did to shoplazza.theme.toml.
type envWriteAction string

const (
	envWriteCreated   envWriteAction = "created"   // file was absent; wrote it
	envWriteUpdated   envWriteAction = "updated"   // file existed; default env rewritten
	envWriteUnchanged envWriteAction = "unchanged" // default env already matched
	envWriteSkipped   envWriteAction = "skipped"   // file existed; left untouched
	envWriteNone      envWriteAction = "none"      // nothing worth recording
)

// maybeWriteThemeEnv records the store/theme/profile a pull or push just used
// into shoplazza.theme.toml, so the theme directory is immediately -e aware and
// the chosen target sticks. It writes to the selected environment when -e is
// given, else to "default". tag ("pull"/"push") only labels the stderr notes.
// Best-effort: every failure degrades to a stderr note and never fails the
// (already-successful) command.
//
// Semantics mirror the store-file/config asymmetry: an absent file is created
// outright, but an existing file is user-owned config (it may carry other
// hand-authored environments and comments) so it is only rewritten after a human
// confirms — agents are handed the exact `env set` command instead of a silent
// overwrite.
func maybeWriteThemeEnv(cmd *cobra.Command, f *cmdutil.Factory, cwd string, rs resolvedStore, themeID, tag string) {
	target := environmentName(cmd)
	if target == "" {
		target = env.DefaultEnvironment
	}
	stderr := cmd.ErrOrStderr()

	var confirm func(string) bool
	if cmdutil.Interactive(f) {
		confirm = func(prompt string) bool {
			ok, err := interact.Confirm(prompt)
			return err == nil && ok
		}
	}

	p, action, err := recordThemeEnvironment(cwd, target, rs.Domain, themeID, rs.Profile, confirm)
	if err != nil {
		_, _ = fmt.Fprintf(stderr, "[%s] left %s untouched: %v\n", tag, env.FileName, err)
		return
	}
	switch action {
	case envWriteCreated:
		_, _ = fmt.Fprintf(stderr, "[%s] wrote %s (environment: %s)\n", tag, p, target)
	case envWriteUpdated:
		_, _ = fmt.Fprintf(stderr, "[%s] updated %s (environment: %s)\n", tag, p, target)
	case envWriteSkipped:
		// Existing file, not rewritten. Agents get the command to record it by hand.
		if confirm == nil {
			_, _ = fmt.Fprintf(stderr,
				"[%s] %s already has environment %q; not modifying it. To record this target:\n  shoplazza themes env set %s --store %s --theme %s\n",
				tag, env.FileName, target, target, rs.Domain, themeID)
		} else {
			_, _ = fmt.Fprintf(stderr, "[%s] left %s unchanged\n", tag, env.FileName)
		}
	case envWriteUnchanged, envWriteNone:
	}
}

// recordThemeEnvironment upserts the named environment in shoplazza.theme.toml
// found up from cwd, pinning its store/theme/profile. It is pure of
// terminal/cobra wiring so it is unit-testable: confirm==nil means
// "non-interactive" — an existing file is never rewritten. Returns the file path
// acted on and what happened.
func recordThemeEnvironment(cwd, target, store, themeID, profile string, confirm func(string) bool) (string, envWriteAction, error) {
	if store == "" && profile == "" {
		return "", envWriteNone, nil // nothing bindable to record
	}
	newEnv := env.Environment{Store: store, Theme: themeID, Profile: profile}

	file, p, existed, err := loadOrNewThemeEnvFile(cwd)
	if err != nil {
		return "", envWriteNone, err
	}

	if !existed {
		created := env.File{Environments: map[string]env.Environment{target: newEnv}}
		if serr := env.Save(p, created); serr != nil {
			return "", envWriteNone, serr
		}
		return p, envWriteCreated, nil
	}

	cur, has := file.Environment(target)
	if has && cur.Store == store && cur.Theme == themeID && cur.Profile == profile {
		return p, envWriteUnchanged, nil
	}
	detail := fmt.Sprintf("store=%s, theme=%s", store, themeID)
	if profile != "" {
		detail += ", profile=" + profile
	}
	prompt := fmt.Sprintf("Update the %q environment in %s to %s?", target, env.FileName, detail)
	if confirm == nil || !confirm(prompt) {
		return p, envWriteSkipped, nil
	}
	// Merge onto any existing block so hand-set path/ignore/config survive.
	merged := cur
	merged.Store = store
	merged.Theme = themeID
	merged.Profile = profile
	if file.Environments == nil {
		file.Environments = map[string]env.Environment{}
	}
	file.Environments[target] = merged
	if serr := env.Save(p, file); serr != nil {
		return "", envWriteNone, serr
	}
	return p, envWriteUpdated, nil
}

// classifyPullDownloadErr maps a download stream failure to the right envelope.
// Every branch returns an *output.ExitError: a bare error escaping RunE would
// be reported as a usage error.
func classifyPullDownloadErr(err error, themeID string) error {
	var he *client.HTTPError
	if errors.As(err, &he) {
		switch he.StatusCode {
		case http.StatusNotFound:
			return theme.ErrValidation("theme not found: %s (run `shoplazza themes list` to see available IDs)", themeID)
		case http.StatusUnauthorized, http.StatusForbidden:
			return theme.ErrAuthExpired(err)
		default:
			// status / request id / endpoint ride along for triage.
			return output.ErrAPI(he.StatusCode, he.Body, he.RequestID).WithEndpoint(he.Method, he.Path)
		}
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return theme.ErrLocalIO("download theme zip", err)
}
