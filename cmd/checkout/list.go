package checkout

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	var all bool
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List checkout extensions",
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			params := map[string]any{}
			if !all {
				params["status"] = "published" // default: published only
			}
			return fireAndPrint(cmd, f, client.RawRequest{
				Method: "GET",
				Path:   "/openapi/checkout_extensions/list",
				Params: params,
			})
		},
	}
	cmd.Flags().BoolVarP(&all, "all", "a", false, "List all extensions (not just published)")
	addDryRunFlag(cmd) // no --store-domain: list always acts on the current store
	return cmd
}
