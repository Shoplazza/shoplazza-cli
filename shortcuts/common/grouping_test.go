package common_test

import (
	"context"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func noopExec(context.Context, common.ExecInput) (common.ExecResult, error) {
	return common.ExecResult{}, nil
}

// A shortcut mounted under a module that opts into help grouping is tagged into
// the shortcut/dev tier, so `--help` lists it under that group.
func TestMount_TagsShortcutUnderGroupedParent(t *testing.T) {
	parent := &cobra.Command{Use: "themes"}
	parent.AddGroup(&cobra.Group{ID: cmdutil.GroupShortcut, Title: "Theme development:"})
	common.Mount(common.Shortcut{Service: "themes", Command: "push", Use: "push", Short: "x", Execute: noopExec}, parent, newFakeFactory(t))

	c := onlyChild(t, parent)
	if c.GroupID != cmdutil.GroupShortcut {
		t.Errorf("shortcut under a grouped parent must be tagged %q, got %q", cmdutil.GroupShortcut, c.GroupID)
	}
}

// Under a parent without groups, the shortcut gets no GroupID (so cobra never
// warns about an undefined group and the flat list is preserved).
func TestMount_NoGroupUnderUngroupedParent(t *testing.T) {
	parent := &cobra.Command{Use: "orders"}
	common.Mount(common.Shortcut{Service: "orders", Command: "+x", Use: "+x", Short: "x", Execute: noopExec}, parent, newFakeFactory(t))

	if c := onlyChild(t, parent); c.GroupID != "" {
		t.Errorf("shortcut under an ungrouped parent must have no GroupID, got %q", c.GroupID)
	}
}

func onlyChild(t *testing.T, parent *cobra.Command) *cobra.Command {
	t.Helper()
	subs := parent.Commands()
	if len(subs) != 1 {
		t.Fatalf("want exactly 1 mounted command, got %d", len(subs))
	}
	return subs[0]
}
