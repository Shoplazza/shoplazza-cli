package theme_extension

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	te "github.com/Shoplazza/shoplazza-cli/internal/theme_extension"
)

func newCmdRelease(f *cmdutil.Factory) *cobra.Command {
	var version, path string
	var debug bool
	var cfg te.Config // validated by PreRunE; RunE reuses it (no lossy re-read)
	var target string // resolved target version (flag, else config's recorded version)
	cmd := &cobra.Command{
		Use:   "release",
		Short: "Publish a version in the BOUND APP (partner-openapi, app-token)",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			c, exErr := te.RequireExtensionID(path)
			if exErr != nil {
				return exErr
			}
			if c.ClientID == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"this extension is not bound to an app", "run 'te connect --client-id <id>' first")
			}
			cfg = c
			// Default to the version `te build` recorded; --version overrides it.
			target = version
			if target == "" {
				target = cfg.Version
			}
			if target == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"no version to release (extension.config.json has no version)",
					"run 'shoplazza te build --version <ver>' first, or pass --version <ver> (see 'shoplazza te versions')")
			}
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			// Per-step progress to stderr so the result JSON on stdout stays pipe-clean.
			prog := output.NewProgress(cmd.ErrOrStderr())

			d, err := dashboardClient(ctx, f)
			if err != nil {
				return err
			}

			// Step 1: resolve the bound app's partner + config. partner comes from
			// the binding `te connect` persisted; older configs predate partnerId,
			// so fall back to deriving it from the client_id via /info.
			appStep := prog.Begin("[release] resolving app config")
			pid := cfg.PartnerID
			if pid == "" {
				info, iErr := d.GetCompleteInfo(ctx, cfg.ClientID)
				if iErr != nil {
					appStep.Fail()
					return apiError(iErr)
				}
				pid = string(info.Partner.ID)
				if pid == "" {
					appStep.Fail()
					return output.ErrInternal("could not resolve the bound app's partner from client_id %s", cfg.ClientID)
				}
			}
			appCfg, err := d.GetAppConfig(ctx, pid, cfg.ClientID)
			if err != nil {
				appStep.Fail()
				return apiError(err)
			}
			appStep.Done()

			// Step 2: resolve the semver to its server version_id. The version list
			// is a store-openapi call (store-token), so build a store client for it;
			// the publish below still goes through the app token.
			verStep := prog.Begin("[release] fetching version list")
			store, _, scErr := storeClient(ctx, f, "")
			if scErr != nil {
				verStep.Fail()
				return scErr
			}
			if debug {
				store.Debug = cmd.ErrOrStderr()
			}
			vers, vErr := te.ListVersions(ctx, store, cfg.ExtensionID)
			if vErr != nil {
				verStep.Fail()
				return vErr
			}
			versionID, ok := te.VersionIDFor(vers, target)
			if !ok {
				verStep.Fail()
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"version "+target+" not found for this extension",
					"run 'shoplazza te versions' to list available versions")
			}
			verStep.Done()

			// --debug: surface exactly what we resolved and will POST, so a
			// "version has no doc" / mismatch can be compared against v1's request.
			if debug {
				fmt.Fprintf(cmd.ErrOrStderr(),
					"[debug] publish: POST %s%s\n         extension_id=%s  version=%s  version_id=%s  partner_id=%s  client_id=%s\n",
					f.AuthClient.BaseURL, te.PartnerPublicationsPath,
					cfg.ExtensionID, target, versionID, pid, cfg.ClientID)
			}

			// Step 3: publish the version in the bound app (partner-openapi, app token).
			pubStep := prog.Begin("[release] publishing version " + target + " in app")
			pc, err := partnerOpenapiClient(ctx, f, cfg.ClientID, appCfg.ClientSecret, appCfg.PartnerID, f.AuthClient.BaseURL)
			if err != nil {
				pubStep.Fail()
				return err
			}
			if debug {
				pc.Debug = cmd.ErrOrStderr()
			}
			if pErr := te.Publish(ctx, pc, te.PartnerPublicationsPath, cfg.ExtensionID, versionID); pErr != nil {
				pubStep.Fail()
				return pErr
			}
			pubStep.Done()

			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"extension_id": cfg.ExtensionID, "version": target, "version_id": versionID, "released_in_app": cfg.ClientID}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Version to publish, e.g. 1.0.0 (defaults to the version recorded by 'te build')")
	cmd.Flags().StringVar(&path, "path", ".", "te project root")
	cmd.Flags().BoolVar(&debug, "debug", false, "Print the resolved publish request (endpoint / extension_id / version_id) for troubleshooting")
	return cmd
}
