package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func newCmdVersions(f *cmdutil.Factory) *cobra.Command {
	var extID string
	cmd := &cobra.Command{
		Use:     "versions",
		Short:   "List versions of a checkout extension (for --version-id)",
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extID == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--extension-id is required", "run 'shoplazza checkout list' to find the extension id")
			}
			return fireAndPrint(cmd, f, client.RawRequest{
				Method: "GET",
				Path:   "/openapi/checkout_extensions/version/list",
				Params: map[string]any{"extension_id": extID},
			})
		},
	}
	cmd.Flags().StringVar(&extID, "extension-id", "", "Server-side extension id (from 'checkout list')")
	addDryRunFlag(cmd) // no --store-domain: versions acts on the current store
	return cmd
}
