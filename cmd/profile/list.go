package profile

import (
	internalauth "shoplazza-cli-v2/internal/auth"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:   "list",
		Short: "List all configured profiles",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			rows := internalauth.ProfileRows(f.Config, internalauth.AuthDir(f.ConfigPath))
			items := make([]any, len(rows))
			for i, r := range rows {
				items[i] = r
			}
			return output.PrintBody(cmd.OutOrStdout(), items, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	return cmd
}
