// Package themeext implements the top-level `theme-extension` command
// group (alias `te`): a Go port of v1's theme-extension module. Hand-written
// (mirrors cmd/checkout, cmd/app); MUST NOT be auto-registered via dynamic or
// shortcuts.
package themeext

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func NewCmdThemeExtension(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "theme-extension",
		Aliases: []string{"te"},
		Short:   "Develop, build and deploy Shoplazza theme extensions",
		Long: `Build, run, and ship a Shoplazza theme extension.

Prerequisite:
  shoplazza auth login                    authenticate your partner account

Typical workflow:
  shoplazza theme-extension create        1. scaffold a project (basic | embed)
  shoplazza theme-extension connect       2. link it to an app
  shoplazza theme-extension serve         3. dev: push a build, then sync each saved file
  shoplazza theme-extension build         4. build a new version
  shoplazza theme-extension release       5a. publish the version in the bound app
  shoplazza theme-extension deploy        5b. enable the version in the current store

Manage:  list · versions.  Alias: te.
Run any subcommand with --help for its own flags and examples.`,
	}
	cmd.AddCommand(
		newCmdCreate(f),
		newCmdServe(f),
		newCmdBuild(f),
		newCmdVersions(f),
		newCmdDeploy(f),
		newCmdList(f),
		newCmdConnect(f),
		newCmdRelease(f),
	)
	return cmd
}
