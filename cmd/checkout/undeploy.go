package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

func newCmdUndeploy(f *cmdutil.Factory) *cobra.Command {
	var extID string
	cmd := &cobra.Command{
		Use:     "undeploy",
		Short:   "Undeploy an extension (extension-level, no confirmation)",
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extID == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--extension-id is required", "run 'shoplazza checkout list' to find the extension id")
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
