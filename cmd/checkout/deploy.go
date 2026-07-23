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
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extID == "" || version == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--extension-id and --version are required (deploy is version-level)",
					"run 'shoplazza checkout versions --extension-id <id>' to list versions")
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
