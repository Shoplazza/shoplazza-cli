package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func newCmdDeploy(f *cmdutil.Factory) *cobra.Command {
	var extID, version string
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Activate a previously pushed extension version",
		Long: `Activate (deploy) a specific extension version that was created via
'shoplazza checkout push'. Requires --extension-id and --version.`,
		Example: "  shoplazza checkout-extension deploy --extension-id <id> --version 1.0",
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// A human picks the extension then its version (both from the server);
			// non-interactive callers must pass --extension-id and --version.
			if err := cmdutil.ResolveFlags(cmd, f,
				cmdutil.PromptField{Flag: "extension-id", Title: "Extension", Picker: extensionOptions},
				cmdutil.PromptField{Flag: "version", Title: "Version to activate", Picker: versionOptions},
			); err != nil {
				return err
			}
			// --dry-run stays network-free: show the deploy request with the version
			// (resolved to its server id via /version/list at real run time).
			if cmdutil.IsDryRun(cmd) {
				return output.PrintBody(cmd.OutOrStdout(), map[string]any{
					"dry_run": true,
					"request": f.Client.BuildRequestSummary("POST", "/openapi/checkout_extensions/deploy", nil,
						map[string]any{"extension": map[string]any{"extension_id": extID, "version": version}}),
				}, cmdutil.GetFormat(cmd), "")
			}
			// Human-only confirmation: activating a version replaces the extension
			// currently live on the store's checkout. Non-interactive proceeds;
			// --dry-run already returned above.
			if err := cmdutil.ConfirmDestructive(f,
				"Activate version "+version+" of extension "+extID+"? It replaces the currently live checkout extension."); err != nil {
				return err
			}
			// Resolve the human version (e.g. 1.0) to its server id.
			versionID, exitErr := resolveCheckoutVersionID(cmd.Context(), f, extID, version)
			if exitErr != nil {
				return exitErr
			}
			return fireAndPrint(cmd, f, client.RawRequest{
				Method: "POST",
				Path:   "/openapi/checkout_extensions/deploy",
				Data:   map[string]any{"extension": map[string]any{"extension_id": extID, "id": versionID}},
			})
		},
	}
	cmd.Flags().StringVar(&extID, "extension-id", "", "Server-side extension id")
	cmd.Flags().StringVar(&version, "version", "", "Version to activate, e.g. 1.0 (resolved to its server id via 'checkout versions')")
	addDryRunFlag(cmd) // no --store-domain: deploy acts on the current store
	return cmd
}
