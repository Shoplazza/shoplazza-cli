package cmdutil

import "github.com/spf13/cobra"

// This file is the single source of truth for CLI --help command grouping.
// Grouping is purely a help-rendering concern — command names, behavior and JSON
// output are unchanged. The *wiring* that tells cobra lives at each assembly
// point (cmd/root_groups.go for the root; cmd/dynamic for a module; the shortcut
// engine tags mounted shortcuts), but every one of them reads the data here.
//
// There are two independent grouping models:
//   - Module grouping: within one dynamic module's --help, split subcommands by
//     what they act on — the local project vs the store (GroupShortcut/GroupAPI).
//   - Root grouping: the top-level `shoplazza --help`, by domain (business / dev
//     / tooling), keyed by command name.

// ── Module-level grouping (within a dynamic module) ──────────────────────────

const (
	GroupShortcut = "shortcut" // dev-tier commands working on the local project
	GroupAPI      = "api"      // commands operating on the store over the API
)

// ModuleGroups lists the help groups a dynamic module opts into, keyed by module
// name (render order = slice order). A generated leaf is tagged GroupAPI; a
// mounted shortcut is tagged GroupShortcut unless it sets StoreTier, which puts
// it with the store operations. Unlisted modules render a single flat
// "Available Commands" list.
var ModuleGroups = map[string][]*cobra.Group{
	"themes": {
		{ID: GroupShortcut, Title: "Theme development (local files <-> a theme on your store):"},
		{ID: GroupAPI, Title: "Store theme operations (manage the themes on your store):"},
	},
}

// ── Root-level grouping (the top-level `shoplazza --help`) ────────────────────

const (
	RootGroupBusiness = "business"
	RootGroupDev      = "dev"
	RootGroupTools    = "tools"
)

// RootGroups are the top-level help groups in render order — most-used first,
// the setup/tooling catch-all last.
func RootGroups() []*cobra.Group {
	return []*cobra.Group{
		{ID: RootGroupBusiness, Title: "Store & business (operate your store's data):"},
		{ID: RootGroupDev, Title: "Development (build apps, themes and extensions):"},
		{ID: RootGroupTools, Title: "Setup & tooling (auth · config · schema · CLI):"},
	}
}

// RootGroupOf maps a top-level command name to its group. Unlisted commands
// (future additions, or cobra's auto `help`) fall through to "Additional
// Commands" until added here.
var RootGroupOf = map[string]string{
	// Store & business — operate the store's data (agent/merchant daily driver).
	// `api` is the raw-HTTP way to operate the store, so it belongs here too.
	"orders": RootGroupBusiness, "products": RootGroupBusiness, "customers": RootGroupBusiness,
	"discounts": RootGroupBusiness, "shop": RootGroupBusiness, "webhook": RootGroupBusiness,
	"billing": RootGroupBusiness, "api": RootGroupBusiness,
	// Development — build on the platform.
	"app": RootGroupDev, "themes": RootGroupDev, "theme-extension": RootGroupDev,
	"checkout-extension": RootGroupDev,
	// Setup & tooling — everything else.
	"auth": RootGroupTools, "profile": RootGroupTools, "schema": RootGroupTools,
	"doctor": RootGroupTools, "update": RootGroupTools, "completion": RootGroupTools,
	"skills": RootGroupTools,
}
