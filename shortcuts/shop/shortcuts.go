package shop

import "shoplazza-cli-v2/shortcuts/common"

// Shortcuts returns all shop shortcut commands.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		uploadFileShortcut,
	}
}
