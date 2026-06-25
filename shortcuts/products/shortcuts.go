package products

import "shoplazza-cli-v2/shortcuts/common"

// Shortcuts returns all product shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		searchShortcut,
		countShortcut,
		publishShortcutValue,
		unpublishShortcutValue,
		createShortcut,
		setPriceShortcut,
		stockShortcut,
	}
}
