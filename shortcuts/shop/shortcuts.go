package shop

import "github.com/Shoplazza/shoplazza-cli/shortcuts/common"

// Shortcuts returns all shop shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		uploadFileShortcut,
	}
}
