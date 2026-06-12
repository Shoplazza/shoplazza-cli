package products

import (
	"shoplazza-cli-v2/shortcuts/common"
	"shoplazza-cli-v2/shortcuts/products/collections"
)

// Shortcuts returns all product shortcut commands (including the `products collections` subcommand group).
func Shortcuts() []common.Shortcut {
	out := []common.Shortcut{
		searchShortcut,
		countShortcut,
		publishShortcutValue,
		unpublishShortcutValue,
		createShortcut,
		setPriceShortcut,
		stockShortcut,
	}
	out = append(out, collections.Shortcuts()...)
	return out
}
