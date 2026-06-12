package update

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/build"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

// npmPackage is the published package the CLI updates itself from.
const npmPackage = "shoplazza-cli"

// NewCmdUpdate creates the `update` command, which self-updates via `npm install -g shoplazza-cli@latest`.
func NewCmdUpdate(_ *cmdutil.Factory) *cobra.Command {
	var checkOnly bool
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update the CLI to the latest version (via npm)",
		RunE: func(cmd *cobra.Command, _ []string) error {
			out := cmd.OutOrStdout()
			format := cmdutil.GetFormat(cmd)
			current := build.DisplayVersion()

			npmPath, err := exec.LookPath("npm")
			if err != nil {
				return output.ErrWithHint(
					output.ExitValidation, output.TypeValidation,
					"npm not found on PATH",
					"the CLI is distributed via npm — install Node.js (https://nodejs.org), then run 'npm install -g "+npmPackage+"@latest'",
				)
			}

			latest, latestErr := npmLatestVersion(cmd, npmPath)

			if checkOnly {
				body := map[string]any{"current": current, "latest": latest}
				if latestErr != nil {
					body["latest_error"] = latestErr.Error()
				} else {
					body["up_to_date"] = latest == current
				}
				return output.PrintBody(out, body, format, "")
			}

			// If this binary wasn't installed via npm, `npm install -g` places a
			// separate npm-managed copy that may shadow the current one on PATH.
			if exe, exeErr := os.Executable(); exeErr == nil &&
				!strings.Contains(filepath.ToSlash(exe), "node_modules/"+npmPackage) {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"warning: this binary doesn't look like an npm install (%s).\n"+
						"  'npm install -g %s@latest' will install a separate npm-managed copy.\n",
					exe, npmPackage)
			}

			fmt.Fprintf(cmd.ErrOrStderr(), "Updating %s via npm…\n", npmPackage)
			install := exec.CommandContext(cmd.Context(), npmPath, "install", "-g", npmPackage+"@latest")
			// Send npm progress to stderr so stdout stays clean machine-readable JSON.
			install.Stdout = cmd.ErrOrStderr()
			install.Stderr = cmd.ErrOrStderr()
			if runErr := install.Run(); runErr != nil {
				return output.ErrWithHint(
					output.ExitInternal, output.TypeInternal,
					fmt.Sprintf("npm install failed: %s", runErr.Error()),
					"run it manually: npm install -g "+npmPackage+"@latest",
				)
			}

			return output.PrintBody(out, map[string]any{
				"ok":       true,
				"package":  npmPackage,
				"previous": current,
				"latest":   latest,
			}, format, "")
		},
	}
	cmd.Flags().BoolVar(&checkOnly, "check", false, "Only report current and latest versions; don't install")
	return cmd
}

// npmLatestVersion asks the npm registry for the latest published version.
func npmLatestVersion(cmd *cobra.Command, npmPath string) (string, error) {
	out, err := exec.CommandContext(cmd.Context(), npmPath, "view", npmPackage, "version").Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
