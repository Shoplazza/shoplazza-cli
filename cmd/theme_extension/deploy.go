package theme_extension

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

func newCmdDeploy(f *cmdutil.Factory) *cobra.Command {
	var version, path string
	var debug bool
	var cfg te.Config // validated by PreRunE; RunE reuses it (no lossy re-read)
	var target string // resolved target version (flag, else config's recorded version)
	cmd := &cobra.Command{
		Use:   "deploy",
		Short: "Enable a version in the CURRENT STORE (store-token)",
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			c, exErr := te.RequireExtensionID(path)
			if exErr != nil {
				return exErr
			}
			cfg = c
			// Default to the version `te build` recorded in extension.config.json;
			// --version overrides it to deploy a specific build.
			target = version
			if target == "" {
				target = cfg.Version
			}
			if target == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"no version to deploy (extension.config.json has no version)",
					"run 'shoplazza te build --version <ver>' first, or pass --version <ver> (see 'shoplazza te versions')")
			}
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			// deploy always targets the current store (no --store-domain flag);
			// storeClient("") falls back to the current profile's store.
			store, domain, cErr := storeClient(ctx, f, "")
			if cErr != nil {
				return cErr
			}
			if debug {
				store.Debug = cmd.ErrOrStderr()
			}
			// Publish needs the server version_id; resolve it from the semver.
			vers, vErr := te.ListVersions(ctx, store, cfg.ExtensionID)
			if vErr != nil {
				return vErr
			}
			versionID, ok := te.VersionIDFor(vers, target)
			if !ok {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"version "+target+" not found for this extension",
					"run 'shoplazza te versions' to list available versions")
			}
			if pErr := te.Publish(ctx, store, te.StorePublicationsPath, cfg.ExtensionID, versionID); pErr != nil {
				return pErr
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"extension_id": cfg.ExtensionID, "version": target, "version_id": versionID, "enabled_in": domain}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "Version to enable, e.g. 1.0.0 (defaults to the version recorded by 'te build')")
	cmd.Flags().StringVar(&path, "path", ".", "te project root")
	cmd.Flags().BoolVar(&debug, "debug", false, "Print the raw publish request/response for troubleshooting")
	return cmd
}
