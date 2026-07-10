package profile

import (
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// newCmdRename is a skeleton — T11 fills in the RunE body (rename the
// profile plus its keychain entry / metadata file, preserving current/
// previous pointers).
func newCmdRename(f *cmdutil.Factory) *cobra.Command {
	var name, newName string
	cmd := &cobra.Command{
		Use:   "rename",
		Short: "Rename a profile",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			return output.ErrInternal("not implemented")
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Existing profile name (required)")
	cmd.Flags().StringVar(&newName, "new-name", "", "New profile name (required)")
	return cmd
}
