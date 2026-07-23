// Package checkout implements the `shoplazza checkout-extension` command group
// (alias `checkout`): the build/dev toolchain plus the extension lifecycle.
package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
)

// NewCmdCheckout creates the checkout command group.
func NewCmdCheckout(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout-extension",
		Aliases: []string{"checkout"},
		Short:   "Build, develop and manage Shoplazza checkout extensions",
	}
	cmd.AddCommand(
		newCmdBuild(f),
		newCmdDev(f),
		newCmdList(f),
		newCmdVersions(f),
		newCmdPreview(f),
		newCmdDeploy(f),
		newCmdUndeploy(f),
		newCmdInit(f),
		newCmdExtensionCreate(f),
		newCmdPush(f),
	)
	return cmd
}
