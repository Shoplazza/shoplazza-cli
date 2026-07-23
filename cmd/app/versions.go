package appcmd

import (
	"context"
	"io"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func newCmdVersions(f *cmdutil.Factory) *cobra.Command {
	var clientID, partner, path string
	var offset, limit int
	cmd := &cobra.Command{
		Use:     "versions",
		Short:   "List deployed app versions (paginated)",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			p, err := openProject(path)
			if err != nil {
				return err
			}
			// Both flags are optional and default to the active config,
			// independently: pass --client-id to query another app, --partner to
			// override the partner; whichever you omit falls back to the local
			// config (where partner is now stored alongside client_id).
			cid, pid := clientID, partner
			var cfgErr error
			if cid == "" || pid == "" {
				cfg, cErr := p.ActiveConfig()
				if cErr != nil {
					// Remember the failure: if the flags don't cover the gap, the
					// unreadable config is the real cause — not a missing id.
					cfgErr = cErr
				} else {
					if cid == "" {
						cid = cfg.ClientID
					}
					if pid == "" {
						pid = cfg.PartnerID
					}
				}
			}
			if cid == "" || pid == "" {
				if cfgErr != nil {
					return output.ErrValidation("cannot read active config: %v", cfgErr)
				}
				if cid == "" {
					return output.ErrValidation("no client_id; pass --client-id or run 'shoplazza app config use'")
				}
				return output.ErrValidation("no partner_id; pass --partner or re-run 'shoplazza app config link'")
			}
			d, err := dashboardClient(cmd.Context(), f)
			if err != nil {
				return err
			}
			return runVersionsList(cmd.Context(), d, pid, cid, offset, limit, cmd.OutOrStdout(), cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "App client_id (default: active config)")
	cmd.Flags().StringVar(&partner, "partner", "", "Partner id")
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	cmd.Flags().IntVar(&offset, "offset", 0, "Pagination offset")
	cmd.Flags().IntVar(&limit, "limit", 20, "Pagination limit")
	return cmd
}

func runVersionsList(ctx context.Context, d *app.Dashboard, partnerID, clientID string, offset, limit int, w io.Writer, format, jq string) error {
	resp, err := d.GetVersions(ctx, partnerID, clientID, offset, limit)
	if err != nil {
		return apiError(err)
	}
	return output.PrintAPISuccess(w, map[string]any{"versions": resp.Versions, "has_more": resp.HasMore}, format, jq)
}
