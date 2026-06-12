package checkout

import (
	"context"
	"net/url"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/internal/output"
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
		return "", output.Errorf(output.ExitAPI, output.TypeAPI, "server rejected the request: %s", msg)
	}
	checkoutURL := asString(mapField(payload(resp.Body), "checkout_url"))
	if checkoutURL == "" {
		return "", output.ErrInternal("preview response missing checkout_url")
	}
	base, baseErr := url.Parse("https://" + store)
	if baseErr != nil || base == nil {
		return "", output.ErrValidation("invalid store domain %q: %v", store, baseErr)
	}
	u, err := url.Parse(checkoutURL)
	if err != nil {
		return "", output.ErrInternal("invalid checkout_url '%s': %s", checkoutURL, err.Error())
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
		Use:     "preview",
		Short:   "Generate a preview URL for an extension version",
		PreRunE: authPreRun(f),
		RunE: func(cmd *cobra.Command, _ []string) error {
			if extID == "" || version == "" {
				return output.ErrWithHint(output.ExitValidation, output.TypeValidation,
					"--extension-id and --version are required",
					"run 'shoplazza checkout list' then 'shoplazza checkout versions --extension-id <id>'")
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
