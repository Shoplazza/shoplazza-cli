package appcmd

import (
	"context"
	"net/http"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/app"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

func newCmdDeploy(f *cmdutil.Factory) *cobra.Command {
	var (
		path  string
		debug bool
	)
	cmd := &cobra.Command{
		Use:     "deploy",
		Short:   "Build and deploy extensions",
		Args:    cobra.NoArgs,
		PreRunE: func(cmd *cobra.Command, _ []string) error { return requireLogin(cmd.Context(), f) },
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()

			p, err := openProject(path)
			if err != nil {
				return err
			}

			cfg, ex := activeAppConfig(p)
			if ex != nil {
				return ex
			}

			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}
			cid := cfg.ClientID
			pid, ex := ensurePartnerID(ctx, d, cfg)
			if ex != nil {
				return ex
			}

			targetStore, err := resolveTargetStore(f.Config.CurrentStoreDomain())
			if err != nil {
				return err
			}
			store, err := storeClient(ctx, f, targetStore)
			if err != nil {
				return err
			}
			// version/generate needs the numeric store_id; without it the backend
			// defaults to store 0 and 500s. Resolve it here and surface a resolution
			// failure rather than swallowing it (empty store_id → confusing 500).
			storeID, sErr := resolveStoreID(ctx, f, targetStore)
			if sErr != nil {
				return sErr
			}

			// Partner-openapi client (app token + app-client-id header), used by
			// the theme connection + function create/commit legs. GetAppConfig
			// yields the client_secret + partner_id needed for the app-token
			// acquisition chain that partnerOpenapiClient triggers.
			appCfg, err := d.GetAppConfig(ctx, pid, cid)
			if err != nil {
				return apiError(err)
			}
			partner, err := partnerOpenapiClient(ctx, f, cid, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				return err
			}

			locals, scanErr := app.ScanLocalExtensions(p.Root)
			if scanErr != nil {
				return scanErr
			}
			// v1-compat: warn + drop ids for extensions whose legacy config names a
			// different app than the one being deployed to.
			locals = reconcileExtensionApps(warnWriter(f), locals, cid)

			res, ex := app.Deploy(ctx, app.DeployDeps{
				Dashboard:   d,
				Store:       store,
				Partner:     partner,
				HTTPClient:  &http.Client{Timeout: 60 * time.Second},
				PartnerID:   pid,
				ClientID:    cid,
				StoreID:     storeID,
				ProjectRoot: p.Root,
				Locals:      locals,
				Progress:    output.NewProgress(cmd.ErrOrStderr()),
				BuildArtifact: func(ctx context.Context, l app.LocalExt) (string, *output.ExitError) {
					return app.BuildArtifactFor(ctx, p.Root, l, debug)
				},
			})
			if ex != nil {
				return ex
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), res, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&path, "path", ".", "Project root")
	cmd.Flags().BoolVar(&debug, "debug", false, "Build extensions in debug mode")
	return cmd
}
