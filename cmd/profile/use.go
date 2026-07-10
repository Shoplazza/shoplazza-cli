package profile

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdUse is a skeleton — T11 fills in the RunE body (switch current
// profile, minting a token if none is cached yet).
func newCmdUse(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "use",
		Short: "Switch the current profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.ErrInternal("not implemented")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to switch to (required)")
	return cmd
}
