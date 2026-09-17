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
		"The count endpoint takes no filters — to count a filtered subset, run 'customers +search' " +
		"and read the completeness basis it reports.",
	Example: `  # Total customers
  shoplazza customers +count`,
	Plan: func(_ common.PlanInput) (common.PlannedRequest, error) {
		return PlanCount(map[string]any{}), nil
	},
}
