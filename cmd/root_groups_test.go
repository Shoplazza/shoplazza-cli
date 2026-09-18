package cmd

import (
	"testing"

	"github.com/spf13/cobra"
)

func TestApplyRootGroups(t *testing.T) {
	root := &cobra.Command{Use: "shoplazza"}
	names := []string{
		"orders", "billing", "webhook", // business
		"app", "themes", "checkout-extension", // dev
		"auth", "profile", "schema", "skills", // tools
		"futurecmd", // unmapped → stays ungrouped
	}
	for _, n := range names {
		root.AddCommand(&cobra.Command{Use: n})
	}

	applyRootGroups(root)

	for _, id := range []string{"business", "dev", "tools"} {
		if !root.ContainsGroup(id) {
			t.Errorf("root must declare group %q", id)
		}
	}

	want := map[string]string{
		"orders": "business", "billing": "business", "webhook": "business",
		"app": "dev", "themes": "dev", "checkout-extension": "dev",
		"auth": "tools", "profile": "tools", "schema": "tools", "skills": "tools",
		"futurecmd": "", // unmapped command must not be tagged (no cobra warning)
	}
	for _, c := range root.Commands() {
		if g, ok := want[c.Name()]; ok && c.GroupID != g {
			t.Errorf("%s GroupID=%q, want %q", c.Name(), c.GroupID, g)
		}
	}
}

// Every real top-level command name in rootGroupOf must map to one of the three
// declared groups — guards against a typo'd group id that cobra would warn on.
func TestRootGroupOf_OnlyKnownGroups(t *testing.T) {
	known := map[string]bool{"business": true, "dev": true, "tools": true}
	for name, g := range rootGroupOf {
		if !known[g] {
			t.Errorf("command %q maps to unknown group %q", name, g)
		}
	}
}
