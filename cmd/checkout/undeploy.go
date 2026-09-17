package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func newCmdUndeploy(f *cmdutil.Factory) *cobra.Command {
	var extID string
	cmd := &cobra.Command{
		Use:   "undeploy",
		Short: "Undeploy an extension (extension-level, no confirmation)",
		Long:  "Undeploy a checkout extension on the current store (extension-level, takes effect with no confirmation); requires --extension-id.",
		Example: `  # Preview the undeploy request without sending
  shoplazza checkout-extension undeploy --extension-id ext_123 --dry-run

  # Undeploy an extension
  shoplazza checkout-extension undeploy --extension-id ext_123`,
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extID == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--extension-id is required", "run 'shoplazza checkout list' to find the extension id")
			}
			// Human-only confirmation; agents/pipes/CI proceed unchanged and
			// --dry-run (handled inside fireAndPrint) never reaches here.
			if !cmdutil.IsDryRun(cmd) {
				if err := cmdutil.ConfirmDestructive(f,
					"Undeploy extension "+extID+"? It stops serving on the current store immediately."); err != nil {
					return err
				}
			}
			return fireAndPrint(cmd, f, client.RawRequest{
				Method: "POST",
				Path:   "/openapi/checkout_extensions/undeploy",
				Data:   map[string]any{"extension": map[string]any{"extension_id": extID}},
			})
		},
	}
	cmd.Flags().StringVar(&extID, "extension-id", "", "Server-side extension id")
	addDryRunFlag(cmd) // no --store-domain: undeploy acts on the current store
	return cmd
}
