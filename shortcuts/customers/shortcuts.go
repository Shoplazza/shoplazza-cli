package customers

import "shoplazza-cli-v2/shortcuts/common"

// Shortcuts returns all customers shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		searchShortcut,
		createShortcut,
	}
}
