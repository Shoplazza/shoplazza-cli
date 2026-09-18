package checkoutext

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
	return extractExtensionOptions(resp.Body), nil
}

// extractExtensionOptions maps a checkout list response body ({data:{extensions:
// [{name,extension_id,publish_status}]}}) to picker options: label = name (plus
// publish status), value = extension_id. Rows without an id are skipped; an
// empty name falls back to the id. Pure, so the field mapping is unit-tested.
func extractExtensionOptions(body any) []interact.Option {
	arr, _ := mapField(payload(body), "extensions").([]any)
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
	return opts
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
	return extractVersionOptions(resp.Body), nil
}

// extractVersionOptions maps a checkout version-list response body (the array is
// nested under the key literally named "extensions", each entry {version,id}) to
// picker options: label and value are the version string (deploy/preview resolve
// it to a server id downstream). Pure, so the field mapping is unit-tested.
func extractVersionOptions(body any) []interact.Option {
	arr, _ := mapField(payload(body), "extensions").([]any)
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
	return opts
}
