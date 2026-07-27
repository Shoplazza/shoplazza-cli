package theme_extension

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

func newCmdConnect(f *cmdutil.Factory) *cobra.Command {
	var clientID, path string
	var cfg te.Config // validated by PreRunE; RunE reuses it (no lossy re-read)
	cmd := &cobra.Command{
		Use:   "connect",
		Short: "Link this theme extension to an app (partner-openapi, app-token)",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if clientID == "" {
				return output.ErrValidation("--client-id is required")
			}
			c, exErr := te.RequireExtensionID(path)
			if exErr != nil {
				return exErr // missing/corrupt config → validation + hint (before any network)
			}
			cfg = c
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}
			// client_id uniquely identifies the app; derive its partner via /info
			// (same as te release / app config link) — no --partner needed.
			info, err := d.GetCompleteInfo(ctx, clientID)
			if err != nil {
				return apiError(err)
			}
			pid := string(info.Partner.ID)
			if pid == "" {
				return output.ErrInternal("could not resolve the app's partner from client_id %s", clientID)
			}
			appCfg, err := d.GetAppConfig(ctx, pid, clientID) // yields client_secret (not persisted) + partner_id
			if err != nil {
				return apiError(err)
			}
			pc, err := partnerOpenapiClient(ctx, f, clientID, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				return err
			}
			// Standalone te links at 2024-07 — the SAME API version te release
			// publishes at. (A link under a different version isn't seen by the
			// publish → "version has no doc".)
			if cErr := app.ConnectTheme(ctx, pc, cfg.Name, cfg.ExtensionID, app.ThemeConnectionPathStandalone); cErr != nil {
				return cErr
			}
			// Persist the binding (client_id + its partner) so release knows the app
			// and can skip the partner lookup.
			cfg.ClientID = clientID
			cfg.PartnerID = pid
			if wErr := te.WriteConfig(path, cfg); wErr != nil {
				return output.ErrInternal("connected but failed to persist client_id: %v", wErr)
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"extension_id": cfg.ExtensionID, "client_id": clientID, "connected": true}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&clientID, "client-id", "", "App client_id to link (required)")
	cmd.Flags().StringVar(&path, "path", ".", "te project root")
	return cmd
}
