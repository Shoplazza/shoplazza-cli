// Package theme_extension implements the top-level `theme-extension` command
// group (alias `te`): a Go port of v1's theme-extension module. Hand-written
// (mirrors cmd/checkout, cmd/app); MUST NOT be auto-registered via dynamic or
// shortcuts.
package theme_extension

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func NewCmdThemeExtension(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "theme-extension",
		Aliases: []string{"te"},
		Short:   "Develop, build and deploy Shoplazza theme extensions",
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
