package customers

import "github.com/Shoplazza/shoplazza-cli/shortcuts/common"

// Shortcuts returns all customers shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		searchShortcut,
		createShortcut,
	}
}
