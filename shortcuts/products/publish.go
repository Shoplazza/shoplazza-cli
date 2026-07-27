package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

var publishShortcutValue = common.Shortcut{
	Service: "products",
	Command: "+publish",
	Use:     "+publish --id <product-id>",
	Short:   "Quickly publish a product",
	Flags: []common.Flag{
		common.IDFlag("Product ID (required)."),
	},
	Plan: func(in common.PlanInput) (common.PlannedRequest, error) {
		body := map[string]any{"product": map[string]any{"published": true}}
		return PlanUpdate(in.Flags.GetString("id"), body), nil
	},
}
