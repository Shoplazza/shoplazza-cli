package theme_extension

import (
	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
	te "shoplazza-cli-v2/internal/theme_extension"
)

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	var storeDomain string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List the store's theme extensions",
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			store, _, cErr := storeClient(ctx, f, storeDomain)
			if cErr != nil {
				return cErr
			}
			exts, lErr := te.ListExtensions(ctx, store)
			if lErr != nil {
				return lErr
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"extensions": exts}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Target store (defaults to current store)")
	return cmd
}
