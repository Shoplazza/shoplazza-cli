package themecmd

import (
	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// RegisterCommands mounts the plain-cobra theme workflow commands under the
// existing top-level `themes` command (created by the dynamic module and shared
// with the theme CRUD leaves). Call it after the dynamic + shortcut registration
// so `themes` and its help groups already exist. A no-op if `themes` is absent.
func RegisterCommands(root *cobra.Command, f *cmdutil.Factory) {
	themes := childNamed(root, "themes")
	if themes == nil {
		return
	}
	for _, c := range []*cobra.Command{newCmdPush(f), newCmdPull(f), newCmdServe(f), newCmdShare(f)} {
		// Tag as the dev/shortcut tier when the module groups its help, matching
		// how the shortcut engine tags mounted shortcuts.
		if themes.ContainsGroup(cmdutil.GroupShortcut) {
			c.GroupID = cmdutil.GroupShortcut
		}
		themes.AddCommand(c)
	}
}

func childNamed(parent *cobra.Command, name string) *cobra.Command {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return c
		}
	}
	return nil
}
