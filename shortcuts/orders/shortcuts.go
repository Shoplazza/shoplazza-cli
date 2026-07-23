package orders

import "github.com/Shoplazza/shoplazza-cli/shortcuts/common"

// Shortcuts returns all orders shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		searchShortcut,
		countShortcut,
		shipShortcut,
		refundShortcut,
		updateTrackingShortcut,
	}
}
