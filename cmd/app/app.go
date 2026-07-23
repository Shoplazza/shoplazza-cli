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
