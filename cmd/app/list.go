package appcmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/app"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
)

func newCmdList(f *cmdutil.Factory) *cobra.Command {
	var partner string
	cmd := &cobra.Command{
		Use:     "list",
		Short:   "List apps in your account (partner / client_id / name)",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			return runList(cmd.Context(), d, partner, cmd.OutOrStdout(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&partner, "partner", "", "Partner id (required when the account has multiple partners)")
	return cmd
}

func runList(ctx context.Context, d *app.Dashboard, partnerFlag string, w io.Writer, format, jq string) error {
	partners, err := d.GetPartners(ctx)
	if err != nil {
		return apiError(err)
	}
	if len(partners.Partners) == 0 {
		return output.ErrValidation("no partners available for this account")
	}
	// list is the discovery command: with no --partner it lists apps across ALL
	// partners (a circular "run app list to pick a partner" hint would be useless);
	// --partner filters to one.
	matched := false
	rows := make([]map[string]any, 0)
	for _, p := range partners.Partners {
		pid := string(p.ID)
		if partnerFlag != "" && pid != partnerFlag {
			continue
		}
		matched = true
		apps, aErr := d.GetApps(ctx, pid)
		if aErr != nil {
			return apiError(aErr)
		}
		for _, a := range apps.Apps {
			rows = append(rows, map[string]any{"partner": pid, "id": string(a.ID), "client_id": a.ClientID, "name": a.Name})
		}
	}
	if partnerFlag != "" && !matched {
		return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
			"partner '"+partnerFlag+"' not found in your account",
			"run 'shoplazza app list' without --partner to see all partners")
	}
	return output.PrintAPISuccess(w, map[string]any{"apps": rows, "total": len(rows)}, format, jq)
}
