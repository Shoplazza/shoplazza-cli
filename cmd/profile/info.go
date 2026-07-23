package profile

import (
	internalauth "github.com/Shoplazza/shoplazza-cli/internal/auth"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"

	"github.com/spf13/cobra"
)

func newCmdInfo(f *cmdutil.Factory) *cobra.Command {
	var name string
	cmd := &cobra.Command{
		Use:   "info",
		Short: "Show a profile's details (defaults to the current profile)",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			target, err := currentOrNamed(f, name)
			if err != nil {
				return err
			}
			p := f.Config.FindProfile(target)
			if p == nil {
				return output.ErrValidation("profile %q not found", target)
			}

			// Shared display row plus the info-only fields.
			row, meta := internalauth.ProfileRow(f.Config, internalauth.AuthDir(f.ConfigPath), *p)
			row["account"] = p.Account
			row["token_expiry"] = meta.ExpiresAt
			return output.PrintBody(cmd.OutOrStdout(), row, cmdutil.GetFormat(cmd), cmdutil.GetJQ(cmd))
		},
	}
	cmd.Flags().StringVar(&name, "name", "", "Profile to show (defaults to the current profile)")
	return cmd
}
