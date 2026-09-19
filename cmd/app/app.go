package appcmd

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// NewCmdApp creates the app command group (hand-written, bare name; mirrors
// cmd/checkout and cmd/auth). MUST NOT be auto-registered via dynamic/shortcuts.
func NewCmdApp(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "app",
		Short: "Create, develop and deploy Shoplazza apps and their extensions",
		Long: `Create, develop, and ship a Shoplazza app and its extensions.

Prerequisites:
  shoplazza auth login              authenticate your partner account
  cd <workspace>                    a directory to hold the app project

Typical workflow:
  shoplazza app init                1. create or link an app project
  shoplazza app dev                 2. run it locally (auto tunnel; re-run to apply changes)
  shoplazza app extension --help    3. add an extension (checkout / theme / function)
  shoplazza app deploy              4. build and deploy a new version
  shoplazza app versions            5. list deployed versions

Inspect / configure:
  shoplazza app info                show the current app and its extensions
  shoplazza app config --help       manage and switch the active app config

Run any subcommand with --help for its own flags and examples.`,
	}
	cmd.AddCommand(
		newCmdInit(f),
		newCmdList(f),
		newCmdInfo(f),
		newCmdConfig(f),
		newCmdExtension(f),
		newCmdVersions(f),
		newCmdDeploy(f),
		newCmdDev(f),
		newCmdFunction(f),
	)
	return cmd
}
