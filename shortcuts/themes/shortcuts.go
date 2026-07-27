// Package themes hosts the workflow shortcut commands for theme development.
//
// Namespace note: both the workflow shortcuts in this package and the
// spec-driven dynamic CRUD commands mount under top-level `shoplazza themes`
// (Service: "themes"). They share one command group:
//
//	shoplazza themes init/package/pull/push/share/serve   (workflows, this package)
//	shoplazza themes list/get/publish/delete/task/...      (dynamic CRUD, from the v2 spec)
//
// There is no `list` workflow shortcut: listing is provided by the dynamic
// `themes list` command, so the workflow side stays free of a name collision.
package themes

import "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

// Shortcuts returns the registered themes workflow shortcuts. The dynamic CRUD
// commands (themes list / get / publish / delete / ...) are registered
// separately by the dynamic engine from the v2 spec; this slice only contains
// the workflow shortcuts that have no direct spec equivalent.
//
// The 6 workflow shortcuts (init / package / pull / push / share / serve) all
// mount under top-level `themes`; `list` is intentionally absent (the dynamic
// `themes list` covers it).
func Shortcuts() []common.Shortcut {
	return []common.Shortcut{
		initShortcut,
		packageShortcut,
		pullShortcut,
		pushShortcut,
		shareShortcut,
		serveShortcut,
	}
}
