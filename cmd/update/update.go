package update

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/build"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/metasync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/updatecheck"
)

// npmPackage is the published package the CLI updates itself from.
const npmPackage = "shoplazza-cli"

// npmOps abstracts the npm invocations so runUpdate is testable without npm.
type npmOps struct {
	lookPath func() (string, error)
	latest   func(ctx context.Context, npmPath string) (string, error)
	install  func(ctx context.Context, npmPath string, out io.Writer) error
}

func realNpmOps() npmOps {
	return npmOps{
		lookPath: func() (string, error) { return exec.LookPath("npm") },
		latest: func(ctx context.Context, npmPath string) (string, error) {
			out, err := exec.CommandContext(ctx, npmPath, "view", npmPackage, "version").Output()
			if err != nil {
				return "", err
			}
			return strings.TrimSpace(string(out)), nil
		},
		install: func(ctx context.Context, npmPath string, out io.Writer) error {
			c := exec.CommandContext(ctx, npmPath, "install", "-g", npmPackage+"@latest")
			c.Stdout = out
			c.Stderr = out
			return c.Run()
		},
	}
}

// NewCmdUpdate creates the `update` command, which self-updates via `npm install -g shoplazza-cli@latest`.
func NewCmdUpdate(_ *cmdutil.Factory) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the CLI binary (via npm) and refresh the API metadata",
		// Attempts binary self-update.
		Annotations: map[string]string{cmdutil.AnnotationNotScannable: "true"},
		RunE: func(cmd *cobra.Command, _ []string) error {
			return runUpdate(cmd.Context(), cmd.OutOrStdout(), cmd.ErrOrStderr(),
				cmdutil.GetFormat(cmd), build.DisplayVersion(), checkOnly, realNpmOps())
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only report current and latest versions; don't install")
	return cmd
}

// upToDate reports whether current is already at or beyond latest. A failed or
// empty latest lookup returns false: we can't confirm, so allow the update.
func upToDate(latest, current string, latestErr error) bool {
	if latestErr != nil || latest == "" {
		return false
	}
	return !updatecheck.IsNewer(latest, current)
}

// metaRefresh is swappable in tests (the real one hits the network).
var metaRefresh = func(ctx context.Context, version string) (metasync.Result, error) {
	return metasync.ForceRefresh(ctx, version)
}

// metaOutcome is what a refresh attempt has to say for itself. A disabled
// refresh (ran=false) reports nothing at all, not even that it was skipped.
type metaOutcome struct {
	res metasync.Result
	err error
	ran bool
}

// mergeInto adds the outcome to a response body. Error paths whose envelope is
// the error itself simply don't call it.
func (o metaOutcome) mergeInto(body map[string]any) {
	if !o.ran {
		return
	}
	if o.err != nil {
		body["meta_error"] = o.err.Error()
		return
	}
	body["meta_updated"] = o.res.Updated
	if o.res.Updated {
		body["meta_revision"] = o.res.NewRevision
	} else {
		body["meta_revision"] = o.res.OldRevision
	}
}

// refreshMetadata force-refreshes the OpenAPI metadata cache; version is the
// CLI version that will run next. Honors the disable env (ForceRefresh itself
// does not); failures never affect the exit code.
func refreshMetadata(ctx context.Context, version string) metaOutcome {
	if os.Getenv(metasync.EnvDisable) != "" {
		return metaOutcome{}
	}
	res, err := metaRefresh(ctx, version)
	return metaOutcome{res: res, err: err, ran: true}
}

// runUpdate performs the two halves of `update` independently: the npm binary
// update and the metadata refresh. A failed binary half (npm missing, install
// error) still refreshes metadata before returning its error.
func runUpdate(ctx context.Context, out, errW io.Writer, format, current string, checkOnly bool, ops npmOps) error {
	npmPath, err := ops.lookPath()
	if err != nil {
		if !checkOnly {
			refreshMetadata(ctx, current)
		}
		return output.ErrWithHint(
			output.ExitValidation, output.TypeValidation,
			"npm not found on PATH",
			"the CLI is distributed via npm — install Node.js (https://nodejs.org), then run 'npm install -g "+npmPackage+"@latest'",
		)
	}

	latest, latestErr := ops.latest(ctx, npmPath)

	if checkOnly {
		body := map[string]any{"current": current, "latest": latest}
		if latestErr != nil {
			body["latest_error"] = latestErr.Error()
		} else {
			body["up_to_date"] = upToDate(latest, current, nil)
		}
		return output.PrintBody(out, body, format, "")
	}

	if upToDate(latest, current, latestErr) {
		fmt.Fprintf(errW, "✓ %s is already up to date (%s)\n", npmPackage, current)
		body := map[string]any{
			"ok": true, "package": npmPackage, "current": current, "latest": latest, "updated": false,
		}
		meta := refreshMetadata(ctx, current)
		meta.mergeInto(body)
		return output.PrintBody(out, body, format, "")
	}

	// A non-npm binary would be shadowed by the separate npm-managed copy on PATH.
	if exe, exeErr := os.Executable(); exeErr == nil &&
		!strings.Contains(filepath.ToSlash(exe), "node_modules/"+npmPackage) {
		fmt.Fprintf(errW,
			"warning: this binary doesn't look like an npm install (%s).\n"+
				"  'npm install -g %s@latest' will install a separate npm-managed copy.\n",
			exe, npmPackage)
	}

	// Capture npm output so its deprecation warnings don't clutter the spinner;
	// surface it only on failure.
	var npmOut bytes.Buffer
	step := output.NewProgress(errW).Begin("Updating " + npmPackage)
	if runErr := ops.install(ctx, npmPath, &npmOut); runErr != nil {
		step.Fail()
		if npmOut.Len() > 0 {
			fmt.Fprintln(errW, strings.TrimRight(npmOut.String(), "\n"))
		}
		refreshMetadata(ctx, current)
		return output.ErrWithHint(
			output.ExitInternal, output.TypeInternal,
			fmt.Sprintf("npm install failed: %s", runErr.Error()),
			"run it manually: npm install -g "+npmPackage+"@latest",
		)
	}
	step.Done()

	newVersion := latest
	if newVersion == "" {
		if v, vErr := ops.latest(ctx, npmPath); vErr == nil {
			newVersion = v
		}
	}
	// npm installed something but neither lookup said what. Fall back to the
	// version we came from, since the notice and the metadata probe both need
	// one; the body keeps "" to report the lookup as unknown.
	installed := newVersion
	if installed == "" {
		installed = current
	}
	fmt.Fprintf(errW, "✓ Updated %s %s → %s\n", npmPackage, current, installed)

	body := map[string]any{
		"ok": true, "package": npmPackage, "previous": current, "latest": newVersion, "updated": true,
	}
	meta := refreshMetadata(ctx, installed)
	meta.mergeInto(body)
	return output.PrintBody(out, body, format, "")
}
