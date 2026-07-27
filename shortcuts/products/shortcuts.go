package products

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

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
		tagShortcut,
	}
}
