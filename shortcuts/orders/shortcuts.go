package orders

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

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
