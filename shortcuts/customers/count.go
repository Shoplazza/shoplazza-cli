package customers

import (
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var countShortcut = common.Shortcut{
	Service: "customers",
	Command: "+count",
	Use:     "+count",
	Short:   "Quickly count customers",
	Long: "Return the total number of customers in the store, without fetching the rows. " +
		"The count endpoint takes no filters — to count a filtered subset, page through " +
		"'customers +search' with the filter (--page-limit 250, then --cursor while .data.has_more is true) and count the rows.",
	Example: `  # Total customers
  shoplazza customers +count`,
	Plan: func(_ common.PlanInput) (common.PlannedRequest, error) {
		return PlanCount(map[string]any{}), nil
	},
}
