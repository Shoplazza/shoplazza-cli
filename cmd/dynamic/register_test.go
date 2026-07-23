package dynamic

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"

	"github.com/spf13/cobra"
)

func newFactory(t *testing.T) *cmdutil.Factory {
	t.Helper()
	return &cmdutil.Factory{}
}

func hasSubcommand(parent *cobra.Command, name string) bool {
	for _, c := range parent.Commands() {
		if c.Name() == name {
			return true
		}
	}
	return false
}

func TestRegisterCommands_NilSpec(t *testing.T) {
	root := &cobra.Command{}
	RegisterCommands(root, nil, newFactory(t))
	if len(root.Commands()) != 0 {
		t.Fatalf("nil spec must register nothing, got %d", len(root.Commands()))
	}
}

func TestRegisterCommands_EmptyModules(t *testing.T) {
	root := &cobra.Command{}
	RegisterCommands(root, &registry.Spec{}, newFactory(t))
	if len(root.Commands()) != 0 {
		t.Fatalf("empty modules: got %d cmds", len(root.Commands()))
	}
}

func TestRegisterCommands_SkipsCollidingNames(t *testing.T) {
	root := &cobra.Command{}
	root.AddCommand(&cobra.Command{Use: "auth"})
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "auth", Commands: []registry.Command{
			{Path: []string{"login"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	if cnt := len(root.Commands()); cnt != 1 {
		t.Fatalf("expected built-in auth retained without dup; got %d cmds", cnt)
	}
}

func TestRegisterCommands_SkipsDuplicateModules(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{
		{Name: "orders", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
		{Name: "orders", Commands: []registry.Command{{Path: []string{"b"}, HTTP: registry.HTTP{Method: "GET", Path: "/b"}}}},
	}}
	RegisterCommands(root, spec, newFactory(t))
	var orders int
	for _, c := range root.Commands() {
		if c.Name() == "orders" {
			orders++
		}
	}
	if orders != 1 {
		t.Fatalf("duplicate modules: got %d 'orders' cmds, want 1", orders)
	}
}

func TestRegisterCommands_SkipsBadModuleName(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{
		{Name: "Bad_Name", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
		{Name: "good", Commands: []registry.Command{{Path: []string{"a"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}}}},
	}}
	RegisterCommands(root, spec, newFactory(t))
	if hasSubcommand(root, "Bad_Name") {
		t.Fatal("non-kebab-case module must be skipped")
	}
	if !hasSubcommand(root, "good") {
		t.Fatal("good module should register")
	}
}

func TestRegisterCommands_DuplicatePathSkipsBoth(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "x", Commands: []registry.Command{
			{Path: []string{"dup"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}},
			{Path: []string{"dup"}, HTTP: registry.HTTP{Method: "GET", Path: "/b"}},
			{Path: []string{"keep"}, HTTP: registry.HTTP{Method: "GET", Path: "/c"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	x := root.Commands()[0]
	if hasSubcommand(x, "dup") {
		t.Fatal("duplicate path[] commands must both be skipped")
	}
	if !hasSubcommand(x, "keep") {
		t.Fatal("non-conflicting command must remain")
	}
}

func TestRegisterCommands_PrefixConflictSkipsBoth(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "x", Commands: []registry.Command{
			{Path: []string{"coupons"}, HTTP: registry.HTTP{Method: "GET", Path: "/a"}},
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/b", Body: "*"}},
			{Path: []string{"survivor"}, HTTP: registry.HTTP{Method: "GET", Path: "/c"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	x := root.Commands()[0]
	for _, c := range x.Commands() {
		if c.Name() == "coupons" {
			t.Fatalf("prefix-conflict commands must be skipped, found %q", c.Name())
		}
	}
	if !hasSubcommand(x, "survivor") {
		t.Fatal("survivor must remain")
	}
}

func TestRegisterCommands_ImplicitGroupReuse(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "discounts", Commands: []registry.Command{
			{Path: []string{"coupons", "create"}, HTTP: registry.HTTP{Method: "POST", Path: "/c", Body: "*"}},
			{Path: []string{"coupons", "get"}, HTTP: registry.HTTP{Method: "GET", Path: "/g"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	discounts := root.Commands()[0]
	var coupons *cobra.Command
	for _, c := range discounts.Commands() {
		if c.Name() == "coupons" {
			coupons = c
		}
	}
	if coupons == nil {
		t.Fatal("coupons implicit group should exist")
	}
	if !hasSubcommand(coupons, "create") || !hasSubcommand(coupons, "get") {
		t.Fatal("coupons group must hold both create + get leaves")
	}
}

func TestRegisterCommands_ModuleWithZeroValidCommandsSkipped(t *testing.T) {
	root := &cobra.Command{}
	spec := &registry.Spec{Modules: []registry.Module{{
		Name: "broken", Commands: []registry.Command{
			{Path: []string{"BAD_NAME"}, HTTP: registry.HTTP{Method: "GET", Path: "/x"}},
		},
	}}}
	RegisterCommands(root, spec, newFactory(t))
	if hasSubcommand(root, "broken") {
		t.Fatal("module with 0 valid commands must not appear")
	}
}
