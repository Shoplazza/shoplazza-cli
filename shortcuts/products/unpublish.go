package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

var unpublishShortcutValue = common.Shortcut{
	Service: "products",
	Command: "+unpublish",
	Use:     "+unpublish --id <product-id>",
	Short:   "Quickly unpublish a product",
	Flags: []common.Flag{
		common.IDFlag("Product ID (required)."),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body := map[string]any{"product": map[string]any{"published": false}}
		return PlanUpdate(in.Flags.GetString("id"), body), nil
	},
}
