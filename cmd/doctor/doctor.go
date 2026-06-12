package doctor

import (
	"shoplazza-cli-v2/internal/output"

	"github.com/spf13/cobra"
)

// NewCmdDoctor creates the doctor command group.
func NewCmdDoctor() *cobra.Command {
	cmd := &cobra.Command{
		Use:    "doctor",
		Short:  "Run diagnostic checks",
		Hidden: true,
	}

	cmd.AddCommand(
		newCmdCheck(),
	)

	return cmd
}

func newCmdCheck() *cobra.Command {
	return &cobra.Command{
		Use:   "check",
		Short: "Check current CLI health",
		RunE: func(_ *cobra.Command, _ []string) error {
			return output.ErrWithHint(
				output.ExitInternal,
				output.TypeInternal,
				"doctor check is not yet available",
				"use 'shoplazza auth status' to verify your authentication state",
			)
		},
	}
}
