// Package profile implements "shoplazza profile" — managing named store
// execution contexts (account + store domain + scopes) on top of the v2
// multi-tenant config.
package profile

import (
	"shoplazza-cli-v2/internal/cmdutil"

	"github.com/spf13/cobra"
)

// NewCmdProfile creates the profile command group. The group itself is
// auth-free (it only touches local config/keychain); each subcommand decides
// whether it needs an account or a live exchange.
func NewCmdProfile(f *cmdutil.Factory) *cobra.Command {
	cmd := &cobra.Command{
		Use:         "profile",
		Short:       "Manage store execution contexts (profiles)",
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
	}
	cmd.AddCommand(
		newCmdAdd(f),
		newCmdList(f),
		newCmdShow(f),
		newCmdUse(f),
		newCmdUpdate(f),
		newCmdRename(f),
		newCmdRemove(f),
	)
	return cmd
}
