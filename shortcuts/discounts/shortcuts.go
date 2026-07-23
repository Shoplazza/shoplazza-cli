package discounts

import "github.com/Shoplazza/shoplazza-cli/shortcuts/common"

// Shortcuts returns all discount shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		searchShortcut,
		rebateShortcut,
		flashsaleShortcut,
		mnDiscountShortcut,
		percentCodeShortcut,
		amountCodeShortcut,
		bxgyCodeShortcut,
		freeShippingCodeShortcut,
	}
}
