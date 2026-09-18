package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func newCmdVersions(f *cmdutil.Factory) *cobra.Command {
	var extID string
	cmd := &cobra.Command{
		Use:   "versions",
		Short: "List versions of a checkout extension (for --version-id)",
		Long:  "List the versions of one checkout extension on the current store; requires --extension-id (find it via 'checkout-extension list').",
		Example: `  # List an extension's versions
  shoplazza checkout-extension versions --extension-id ext_123`,
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if err := cmdutil.ResolveFlags(cmd, f,
				cmdutil.PromptField{Flag: "extension-id", Title: "Extension", Picker: extensionOptions},
			); err != nil {
				return err
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
