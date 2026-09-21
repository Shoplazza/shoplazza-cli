// Package themes hosts the page/block editing shortcuts for theme development.
//
// Namespace note: these shortcuts, the plain-cobra dev commands in cmd/theme
// (init / package / pull / push / share / serve / env) and the spec-driven
// dynamic CRUD commands all mount under top-level `shoplazza themes`:
//
//	shoplazza themes +preview                        (preview URL, this package)
//	shoplazza themes +page/+edit                     (page editing, this package)
//	shoplazza themes block +edit/+get                (generated blocks, this package)
//	shoplazza themes init/package/pull/push/...      (dev workflows, cmd/theme)
//	shoplazza themes list/get/publish/delete/...     (dynamic CRUD, from the v2 spec)
package themes

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// Shortcuts returns the registered themes shortcuts. The dynamic CRUD commands
// and the cmd/theme dev workflows are registered separately; this slice only
// contains the page/block shortcuts that have no direct spec equivalent.
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		previewShortcut,
		pageShortcut,
		editShortcut,
		blockEditShortcut,
		blockGetShortcut,
		cardSchemaShortcut,
	}
}
