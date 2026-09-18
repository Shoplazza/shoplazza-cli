package cmd

import "github.com/spf13/cobra"

// rootGroupOf maps a top-level command name to its help group. Unlisted
// commands (e.g. future additions, or cobra's auto `help`) fall through to
// "Additional Commands" until added here.
var rootGroupOf = map[string]string{
	// Store & business — operate the store's data (agent/merchant daily driver).
	"orders": "business", "products": "business", "customers": "business",
	"discounts": "business", "shop": "business", "webhook": "business", "billing": "business",
	// Development — build on the platform.
	"app": "dev", "themes": "dev", "theme-extension": "dev", "checkout-extension": "dev",
	// Setup, access & tooling — everything else.
	"auth": "tools", "profile": "tools", "api": "tools", "schema": "tools",
	"doctor": "tools", "update": "tools", "completion": "tools", "skills": "tools",
}

// applyRootGroups splits the (otherwise flat, 19-command) root help into three
// titled groups — most-frequent first, the setup/tooling catch-all last. Order
// of AddGroup = render order; within a group cobra keeps its default alphabetical
// sort. Help-only: command names/behavior/JSON output are unchanged.
func applyRootGroups(root *cobra.Command) {
	root.AddGroup(
		&cobra.Group{ID: "business", Title: "Store & business (operate your store's data):"},
		&cobra.Group{ID: "dev", Title: "Development (build apps, themes and extensions):"},
		&cobra.Group{ID: "tools", Title: "Setup, access & tooling (auth · config · raw API · CLI):"},
	)
	for _, c := range root.Commands() {
		if g, ok := rootGroupOf[c.Name()]; ok {
			c.GroupID = g
		}
	}
	// Keep cobra's auto `help` command out of a stray "Additional Commands" block.
	root.SetHelpCommandGroupID("tools")
}
