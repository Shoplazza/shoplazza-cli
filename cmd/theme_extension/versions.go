package theme_extension

import (
	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	te "shoplazza-cli-v2/internal/theme_extension"
)

func newCmdVersions(f *cmdutil.Factory) *cobra.Command {
	var path string
	cmd := &cobra.Command{
		Use:     "versions",
		Short:   "List a theme extension's versions",
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			cfg, exErr := te.RequireExtensionID(path)
			if exErr != nil {
				return exErr
			}
			// versions always targets the current store (no --store-domain flag);
			// resolveStore("") falls back to f.Config.StoreDomain.
			domain, sErr := resolveStore(f, "")
			if sErr != nil {
				return sErr
			}
			store, cErr := storeClient(ctx, f, domain)
			if cErr != nil {
				return cErr
			}
			vers, vErr := te.ListVersions(ctx, store, cfg.ExtensionID)
			if vErr != nil {
				return vErr
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"versions": vers}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "te project root")
	return cmd
}
