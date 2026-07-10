package profile

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdUpdate is a skeleton — T11 fills in the RunE body (re-exchange the
// profile's token against the new scopes/store-domain).
func newCmdUpdate(f *cmdutil.Factory) *cobra.Command {
	var (
		name        string
		storeDomain string
		scopes      []string
	)
	cmd := &cobra.Command{
		Use:   "update",
		Short: "Update a profile's store domain or scopes",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.ErrInternal("not implemented")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to update (required)")
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "New store hostname to bind this profile to")
	cmd.Flags().StringSliceVar(&scopes, "scope", nil, "New scopes to request for this profile (must be a subset of the account's granted scopes)")
	return cmd
}
