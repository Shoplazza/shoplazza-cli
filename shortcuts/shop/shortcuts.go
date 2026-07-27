package shop

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// Shortcuts returns all shop shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		uploadFileShortcut,
	}
}
