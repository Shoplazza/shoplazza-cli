package collections

import "shoplazza-cli-v2/shortcuts/common"

// Shortcuts returns all `products collections` shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{createShortcut}
}
