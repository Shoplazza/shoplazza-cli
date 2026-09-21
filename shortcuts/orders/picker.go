package orders

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// orderPicker lets a human who omits --order-id fuzzy-select a recent order.
// The label leads with the order number and financial status (the fields
// +search already exposes via --jq '.data.orders[].number'); the id behind it
// is written onto the flag. Non-interactive callers never see it.
var orderPicker = &common.ResourcePicker{
	Path:  ordersBase,
	Query: map[string]any{"page_size": 50},
	Extract: common.ListExtractor("orders", []string{"id"}, func(o map[string]any) string {
		num := common.FirstString(o, "number", "name", "order_number")
		label := "#" + num
		if num == "" {
			label = common.FirstString(o, "id")
		}
		if st := common.FirstString(o, "financial_status", "status"); st != "" {
			label += " · " + st
		}
		if total := common.FirstString(o, "total_price", "total"); total != "" {
			label += " · " + total
		}
		return label
	}),
}
