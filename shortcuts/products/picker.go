package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// productPicker lets a human who omits --id fuzzy-select a product by title.
// The id behind the chosen title is written onto the flag. Non-interactive
// callers never see it.
var productPicker = &common.ResourcePicker{
	Path:  productsBase,
	Query: map[string]any{"page_size": 50},
	Extract: common.ListExtractor("products", []string{"id"}, func(p map[string]any) string {
		title := common.FirstString(p, "title", "name")
		if st := common.FirstString(p, "status", "published"); st != "" {
			if title == "" {
				return st
			}
			return title + " · " + st
		}
		return title
	}),
}
