package checkout

import (
	"context"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

// extensionOptions lists the store's checkout extensions for a fuzzy picker:
// the label is the extension name (plus publish status) and the value is the
// server-side extension id. Checkout endpoints use their own response shape, so
// this navigates via the local payload/mapField helpers rather than the generic
// resource picker. On any error the caller degrades to manual entry.
func extensionOptions(ctx context.Context, _ *cobra.Command, f *cmdutil.Factory) ([]interact.Option, error) {
	resp, exitErr := doAPI(ctx, f, client.RawRequest{
		Method: "GET",
		Path:   "/openapi/checkout_extensions/list",
	})
	if exitErr != nil {
		return nil, exitErr
	}
	arr, _ := mapField(payload(resp.Body), "extensions").([]any)
	opts := make([]interact.Option, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		id := asString(m["extension_id"])
		if id == "" {
			continue
		}
		label := asString(m["name"])
		if label == "" {
			label = id
		}
		if st := asString(m["publish_status"]); st != "" {
			label += " · " + st
		}
		opts = append(opts, interact.Option{Label: label, Value: id})
	}
	return opts, nil
}

// versionOptions lists one extension's versions for a fuzzy picker. It reads the
// already-resolved --extension-id off cmd, so it must be listed after the
// extension-id field. Label and value are the version string; deploy/preview
// resolve that to a server id downstream via resolveCheckoutVersionID.
func versionOptions(ctx context.Context, cmd *cobra.Command, f *cmdutil.Factory) ([]interact.Option, error) {
	extID, _ := cmd.Flags().GetString("extension-id")
	if extID == "" {
		return nil, nil // no extension chosen yet → fall back to manual entry
	}
	resp, exitErr := doAPI(ctx, f, client.RawRequest{
		Method: "GET",
		Path:   "/openapi/checkout_extensions/version/list",
		Params: map[string]any{"extension_id": extID},
	})
	if exitErr != nil {
		return nil, exitErr
	}
	arr, _ := mapField(payload(resp.Body), "extensions").([]any)
	opts := make([]interact.Option, 0, len(arr))
	for _, it := range arr {
		m, ok := it.(map[string]any)
		if !ok {
			continue
		}
		v := asString(m["version"])
		if v == "" {
			continue
		}
		opts = append(opts, interact.Option{Label: v, Value: v})
	}
	return opts, nil
}
