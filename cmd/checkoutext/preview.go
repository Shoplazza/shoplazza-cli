package checkoutext

import (
	"context"
	"net/url"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// buildPreviewURL POSTs preview and resolves checkout_url against the store base
// + ?step=contact_information. Shared with push.
func buildPreviewURL(ctx context.Context, f *cmdutil.Factory, store, extID, versionID string) (string, *output.ExitError) {
	body := map[string]any{"extension": map[string]any{"extension_id": extID, "id": versionID}}
	resp, exitErr := doAPI(ctx, f, client.RawRequest{
		Method: "POST", Path: "/openapi/checkout_extensions/preview", Data: body,
	})
	if exitErr != nil {
		return "", exitErr
	}
	// Checkout endpoints reject with HTTP 200 + {message, status != 0}; surface
	// the server's message instead of an internal "missing checkout_url".
	if msg := checkoutFailureMessage(resp.Body); msg != "" {
		return "", output.Errorf(output.ExitAPI, output.TypeAPI, "%s", msg).WithRequestID(resp.RequestID())
	}
	checkoutURL := asString(mapField(payload(resp.Body), "checkout_url"))
	if checkoutURL == "" {
		return "", output.ErrInternal("preview response missing checkout_url").WithRequestID(resp.RequestID())
	}
	base, baseErr := url.Parse("https://" + store)
	if baseErr != nil || base == nil {
		return "", output.ErrValidation("invalid store domain %q: %v", store, baseErr)
	}
	u, err := url.Parse(checkoutURL)
	if err != nil {
		return "", output.ErrInternal("invalid checkout_url '%s': %s", checkoutURL, err.Error()).WithRequestID(resp.RequestID())
	}
	if !u.IsAbs() {
		u = base.ResolveReference(u) // resolve relative checkout_url against the store
	}
	q := u.Query()
	q.Set("step", "contact_information")
	u.RawQuery = q.Encode()
	return u.String(), nil
}

func newCmdPreview(f *cmdutil.Factory) *cobra.Command {
	var extID, version string
	cmd := &cobra.Command{
		Use:   "preview",
		Short: "Generate a preview URL for an extension version",
		Long:  "Generate a storefront preview URL for a specific extension version on the current store; requires --extension-id and --version.",
		Example: `  # Preview the plan without calling the API
  shoplazza checkout-extension preview --extension-id ext_123 --version 1.0 --dry-run

  # Generate a preview URL for a version
  shoplazza checkout-extension preview --extension-id ext_123 --version 1.0`,
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			// A human picks the extension then its version (both from the server);
			// non-interactive callers must pass --extension-id and --version.
			if err := cmdutil.ResolveFlags(cmd, f,
				cmdutil.PromptField{Flag: "extension-id", Title: "Extension", Picker: extensionOptions},
				cmdutil.PromptField{Flag: "version", Title: "Version to preview", Picker: versionOptions},
			); err != nil {
				return err
			}
			// preview always targets the current store (no --store-domain override).
			store, exitErr := resolveStore(f)
			if exitErr != nil {
				return exitErr
			}
			// --dry-run stays network-free: show the preview request with the version
			// (resolved to its server id via /version/list at real run time).
			if cmdutil.IsDryRun(cmd) {
				return output.PrintBody(cmd.OutOrStdout(), map[string]any{
					"dry_run": true,
					"request": f.Client.BuildRequestSummary("POST", "/openapi/checkout_extensions/preview", nil,
						map[string]any{"extension": map[string]any{"extension_id": extID, "version": version}}),
				}, cmdutil.GetFormat(cmd), "")
			}
			// Resolve the human version (e.g. 1.0) to its server id.
			versionID, exitErr := resolveCheckoutVersionID(cmd.Context(), f, extID, version)
			if exitErr != nil {
				return exitErr
			}
			previewURL, exitErr := buildPreviewURL(cmd.Context(), f, store, extID, versionID)
			if exitErr != nil {
				return exitErr
			}
			return output.PrintBody(cmd.OutOrStdout(), map[string]any{
				"ok": true, "extension_id": extID, "version": version, "version_id": versionID, "preview_url": previewURL,
			}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVar(&extID, "extension-id", "", "Server-side extension id")
	cmd.Flags().StringVar(&version, "version", "", "Version to preview, e.g. 1.0 (resolved to its server id via 'checkout versions')")
	addDryRunFlag(cmd) // no --store-domain: preview acts on the current store
	return cmd
}
