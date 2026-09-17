package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

var publishShortcutValue = common.Shortcut{
	Service: "products",
	Command: "+publish",
	Use:     "+publish --id <product-id>",
	Short:   "Quickly publish a product",
	Long:    "Make a product visible on the storefront; run --dry-run first to preview the request.",
	Example: `  # Preview publishing a product
  shoplazza products +publish --id 12345 --dry-run

  # Publish it
  shoplazza products +publish --id 12345`,
	Flags: []common.Flag{
		common.IDFlag("Product ID (required).").WithPicker(productPicker),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body := map[string]any{"product": map[string]any{"published": true}}
		return PlanUpdate(in.Flags.GetString("id"), body), nil
	},
}
