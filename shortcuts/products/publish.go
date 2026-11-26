package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

var publishShortcutValue = common.Shortcut{
	Service: "products",
	Command: "+publish",
	Use:     "+publish --product-id <id>",
	Short:   "Quickly publish a product",
	Long:    "Make a product visible on the storefront; run --dry-run first to preview the request.",
	Example: `  # Preview publishing a product
  shoplazza products +publish --product-id 12345 --dry-run

  # Publish it
  shoplazza products +publish --product-id 12345`,
	Flags: []common.Flag{
		common.IDFlag("product", "Product ID (required).").WithPicker(productPicker),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body := map[string]any{"product": map[string]any{"published": true}}
		return PlanUpdate(in.Flags.GetString("product-id"), body), nil
	},
}
