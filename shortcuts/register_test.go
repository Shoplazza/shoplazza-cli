package shortcuts

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"

	"github.com/spf13/cobra"
)

func TestFindOrCreateService_NestedPath(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parent := findOrCreateService(root, "products collections")
	if parent == nil {
		t.Fatal("findOrCreateService returned nil for nested path")
	}
	if parent.Name() != "collections" {
		t.Errorf("leaf name: got %q want %q", parent.Name(), "collections")
	}
	// Verify products exists under root, and collections under products.
	var products *cobra.Command
	for _, c := range root.Commands() {
		if c.Name() == "products" {
			products = c
		}
	}
	if products == nil {
		t.Fatal("products command not created under root")
	}
	var collections *cobra.Command
	for _, c := range products.Commands() {
		if c.Name() == "collections" {
			collections = c
		}
	}
	if collections == nil {
		t.Fatal("collections command not created under products")
	}
	if collections != parent {
		t.Error("returned parent is not the same as the actual collections subcommand")
	}
}

func TestFindOrCreateService_Idempotent(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	a := findOrCreateService(root, "products collections")
	b := findOrCreateService(root, "products collections")
	if a != b {
		t.Error("idempotent: second call should return the same command, not a new one")
	}
}

func TestFindOrCreateService_SingleLevel(t *testing.T) {
	root := &cobra.Command{Use: "root"}
	parent := findOrCreateService(root, "products")
	if parent == nil {
		t.Fatal("nil")
	}
	if parent.Name() != "products" {
		t.Errorf("got %q want products", parent.Name())
	}
}

func TestRegisterShortcuts_MountsCommands(t *testing.T) {
	root := &cobra.Command{Use: "shoplazza"}
	f := &cmdutil.Factory{}
	RegisterShortcuts(root, f)
	if len(root.Commands()) == 0 {
		t.Error("RegisterShortcuts should mount at least one service command")
	}
}
