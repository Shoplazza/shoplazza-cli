package theme_extension

import (
	"errors"
	"net/http"
	"os"
	"time"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/ossupload"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	te "github.com/Shoplazza/shoplazza-cli/v2/internal/theme_extension"
)

func newCmdBuild(f *cmdutil.Factory) *cobra.Command {
	var version, storeDomain, path, description string
	cmd := &cobra.Command{
		Use:   "build",
		Short: "Build a new theme-extension version (zip → OSS → version task)",
		Long:  "Zip theme-app/, upload it, and create a new theme-extension version; --version must be a semver strictly greater than the latest and --description is required.",
		Example: `  # Build a new version 1.0.0
  shoplazza theme-extension build --version 1.0.0 --description "initial release"`,
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			// Interactive fill for the two required values (local text — no server
			// lookup); non-interactively an unset one stays the structured
			// missing-flag error. Auth is unrelated to these and checked last.
			if err := cmdutil.ResolveFlags(cmd, f,
				cmdutil.PromptField{Flag: "version", Title: "New version (semver, e.g. 1.0.0)"},
				cmdutil.PromptField{Flag: "description", Title: "Version description"},
			); err != nil {
				return err
			}
			if !te.ValidVersionFormat(version) {
				return output.ErrValidation("--version must follow X.Y.Z (e.g. 1.0.0)")
			}
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			root := path
			cfg, err := te.ReadConfig(root)
			if err != nil {
				return te.ConfigReadError(root, err)
			}
			store, _, cErr := storeClient(ctx, f, storeDomain)
			if cErr != nil {
				return cErr
			}
			// Per-step progress to stderr so the result JSON on stdout stays pipe-clean.
			prog := output.NewProgress(cmd.ErrOrStderr())
			// The new version must be greater than the latest. A fresh extension (no
			// extension_id) has no versions, so skip. Checked BEFORE zip+upload so a
			// stale version fails fast without a wasted OSS upload.
			if cfg.ExtensionID != "" {
				verStep := prog.Begin("[build] checking latest version")
				vers, vErr := te.ListVersions(ctx, store, cfg.ExtensionID)
				if vErr != nil {
					verStep.Fail()
					return vErr
				}
				verStep.Done()
				// Guard only on a confirmed comparison: a remote latest that isn't
				// strict X.Y.Z makes the ordering inconclusive, so skip the check
				// rather than blocking the build on garbage data.
				if latest := te.LatestVersion(vers); latest != "" && te.ValidVersionFormat(latest) &&
					te.CompareVersions(version, latest) != 1 {
					return output.ErrValidation("--version %s must be greater than the latest version %s", version, latest)
				}
			}
			zipStep := prog.Begin("[build] zipping theme-app")
			zipPath, zErr := te.ZipThemeApp(root)
			if zErr != nil {
				zipStep.Fail()
				if errors.Is(zErr, te.ErrThemeAppMissing) {
					return output.ErrValidation("zip theme-app/: %v (is there a theme-app/ directory?)", zErr)
				}
				return output.ErrInternal("zip theme-app/: %v", zErr)
			}
			zipStep.Done()
			upStep := prog.Begin("[build] uploading theme-app")
			up := &ossupload.Uploader{Client: store, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
			resourceURL, uErr := up.Upload(ctx, zipPath)
			if uErr != nil {
				upStep.Fail()
				return uErr
			}
			upStep.Done()
			// Best-effort removal keeps .te-build/ from growing one zip per build.
			// Kept on failure above for debugging.
			_ = os.Remove(zipPath)
			// Register = PUT theme-extensions + POST version-tasks + poll the task
			// until the server finishes processing the bundle into the version.
			regStep := prog.Begin("[build] creating version " + version + " (server processing)")
			res, rErr := te.Register(ctx, store, root, cfg.Name, resourceURL, version, description, time.Second, 10)
			if rErr != nil {
				regStep.Fail()
				return rErr
			}
			regStep.Done()
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
				"extension_id": res.ExtensionID,
				"version":      version,
				"version_id":   res.ExtensionVersionID,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&version, "version", "", "New semver version (required)")
	cmd.Flags().StringVar(&description, "description", "", "Version description (required)")
	cmd.Flags().StringVarP(&storeDomain, "store-domain", "s", "", "Target store (defaults to current store)")
	cmd.Flags().StringVar(&path, "path", ".", "te project root")
	return cmd
}
