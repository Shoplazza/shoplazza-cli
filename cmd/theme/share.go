package themecmd

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
)

// shopV1 is the v1 shop endpoint share anchors its preview URL at. share stays
// on the v1 path tree (shop + upload) for byte-exact parity with the v1 CLI, so
// existing share-link recipients (often non-CLI tools scraping the URL shape)
// don't break.
const shopV1 = "/openapi/2020-07/shop"

// newCmdShare builds `themes share`: package the current directory and upload it
// as a NEW unpublished preview theme (empty theme_id → fresh slot), then print a
// shareable preview URL. It never overwrites an existing theme — that is
// `themes push`'s job, so a preview can never clobber a real theme.
func newCmdShare(f *cmdutil.Factory) *cobra.Command {
	var environment string
	cmd := &cobra.Command{
		Use:   "share",
		Short: "Upload the current theme as a new temporary preview and print a shareable link",
		Long:  "Package the current theme and upload it as a NEW unpublished preview theme, then print a shareable preview URL; never overwrites an existing theme (use 'themes push' for that).",
		Example: `  # Upload the current theme as a temporary preview and print the link
  shoplazza themes share`,
		// Owns its (env-aware) auth — see push.go.
		Annotations: map[string]string{cmdutil.AnnotationAuthFree: "true"},
		Args:        cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			rs, err := resolveStore(ctx, f, cmd)
			if err != nil {
				return err
			}
			cwd, err := os.Getwd()
			if err != nil {
				return theme.ErrLocalIO("getwd", err)
			}
			name, version, err := theme.ReadInfo(cwd)
			if err != nil {
				return err
			}

			prog := output.NewProgress(cmd.ErrOrStderr())
			// Step 1: v1 /shop for the storefront domain the preview URL anchors at
			// (the API base URL is the gateway, not the storefront).
			shopStep := prog.Begin("[share] fetching shop info")
			shopResp, err := rs.Client.DoRaw(ctx, client.RawRequest{Method: "GET", Path: shopV1})
			if err != nil {
				shopStep.Fail()
				return classifyHTTPErr(err, "")
			}
			storeDomain := extractStoreDomain(asMap(shopResp.Body))
			shopStep.Done()

			// Step 2: pack cwd into a tmp zip (cleaned up on success and failure).
			pkgStep := prog.Begin("[share] packaging theme files")
			zipPath, err := pack.Pack(cwd, theme.ZipName(name, version), pack.PackOptions{})
			if err != nil {
				pkgStep.Fail()
				return theme.ErrLocalIO("pack zip", err)
			}
			defer func() { _ = os.Remove(zipPath) }()
			pkgStep.Done()

			// Step 3: multipart upload (v1 path, empty theme_id → fresh theme) and
			// resolve the id. No fallback: share always creates a theme, so an
			// unresolved id is a hard error.
			id, err := uploadZipResolveThemeID(ctx, rs.Client, prog, "[share]", "", name, version, zipPath, "")
			if err != nil {
				return err
			}
			if id == "" {
				return theme.ErrValidation(
					"server did not return a theme id for the share upload; cannot build a preview URL — retry")
			}

			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{
				"preview_url":  fmt.Sprintf("https://%s/?preview_theme_id=%s", storeDomain, id),
				"theme_id":     id,
				"store_domain": storeDomain,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment from shoplazza.theme.toml (store/profile); see 'themes env list'")
	return cmd
}
