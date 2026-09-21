package cmd

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// applyRootGroups splits the (otherwise flat, 19-command) root help into the
// titled groups defined in internal/cmdutil (the single grouping source of
// truth) — most-frequent first, the setup/tooling catch-all last. Order of
// AddGroup = render order; within a group cobra keeps its default alphabetical
// sort. Help-only: command names/behavior/JSON output are unchanged.
func applyRootGroups(root *cobra.Command) {
	root.AddGroup(cmdutil.RootGroups()...)
	for _, c := range root.Commands() {
		if g, ok := cmdutil.RootGroupOf[c.Name()]; ok {
			c.GroupID = g
		}
	}
	// Keep cobra's auto `help` command out of a stray "Additional Commands" block.
	root.SetHelpCommandGroupID(cmdutil.RootGroupTools)
}
