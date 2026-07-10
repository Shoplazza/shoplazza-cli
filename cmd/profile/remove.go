package profile

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdRemove is a skeleton — T11 fills in the RunE body (drop the profile
// from config plus its keychain token / metadata file, reassigning current/
// previous as needed).
func newCmdRemove(f *cmdutil.Factory) *cobra.Command {
	var name string
	var force bool
	cmd := &cobra.Command{
		Use:   "remove",
		Short: "Remove a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.ErrInternal("not implemented")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to remove (required)")
	cmd.Flags().BoolVar(&force, "force", false, "Skip the confirmation check")
	return cmd
}
