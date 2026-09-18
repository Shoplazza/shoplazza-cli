package cmdutil

// Help-display command groups (cobra GroupID). A module that opts into grouping
// (see cmd/dynamic moduleGroups) splits its subcommands in `--help` into two
// tiers so a reader can tell local/dev commands from store-side API commands:
//
//   - GroupShortcut: shortcut-tier commands mounted from shortcuts/ (e.g. the
//     themes local dev loop: init/pull/serve/push/package/share).
//   - GroupAPI: OpenAPI-generated commands that operate on resources in the
//     store (list/get/publish/delete/...).
//
// The IDs are shared here so the dynamic module (which defines the groups and
// tags generated leaves) and the shortcut engine (which tags mounted shortcuts)
// agree without importing each other. Grouping is purely a help-rendering
// concern — command names, behavior and JSON output are unchanged.
const (
	GroupShortcut = "shortcut"
	GroupAPI      = "api"
)
