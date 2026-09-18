// Package checkoutext implements the `shoplazza checkout-extension` command group
// (alias `checkout`): the build/dev toolchain plus the extension lifecycle.
package checkoutext

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// NewCmdCheckout creates the checkout command group.
func NewCmdCheckout(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:     "checkout-extension",
		Aliases: []string{"checkout"},
		Short:   "Build, develop and manage Shoplazza checkout extensions",
		Long: `Build, run, and ship a Shoplazza checkout extension.

Prerequisite:
  shoplazza auth login                     authenticate your partner account

Typical workflow:
  shoplazza checkout-extension init        1. scaffold a new project (local, no network)
  shoplazza checkout-extension create      2. add an extension to the project (local)
  shoplazza checkout-extension dev         3. run the dev server (rebuild + HMR on :8888)
  shoplazza checkout-extension push        4. build and upload a version (NOT active yet)
  shoplazza checkout-extension deploy      5. activate a pushed version
  shoplazza checkout-extension preview        preview a version before activating

Manage:  list · versions · undeploy.  Alias: checkout.
Run any subcommand with --help for its own flags and examples.`,
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
