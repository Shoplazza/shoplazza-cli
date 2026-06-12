package discounts

import "shoplazza-cli-v2/shortcuts/common"

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
