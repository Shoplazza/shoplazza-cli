package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

var unpublishShortcutValue = common.Shortcut{
	Service: "products",
	Command: "+unpublish",
	Use:     "+unpublish --id <product-id>",
	Short:   "Quickly unpublish a product",

	Destructive:   true,
	ConfirmPrompt: "Unpublish this product? It will be hidden from the storefront.",
	Long:    "Hide a product from the storefront; run --dry-run first to preview the request.",
	Example: `  # Preview unpublishing a product
  shoplazza products +unpublish --id 12345 --dry-run

  # Unpublish it
  shoplazza products +unpublish --id 12345`,
	Flags: []common.Flag{
		common.IDFlag("Product ID (required)."),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body := map[string]any{"product": map[string]any{"published": false}}
		return PlanUpdate(in.Flags.GetString("id"), body), nil
	},
}
