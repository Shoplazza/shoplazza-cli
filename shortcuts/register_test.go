package shortcuts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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

// TestAllShortcutsAreComplete fails if any registered Shortcut has a nil Plan,
// empty Service, or empty Command.
func TestAllShortcutsAreComplete(t *testing.T) {
	if len(allShortcuts) == 0 {
		t.Fatal("allShortcuts is empty; the test would silently pass")
	}
	for _, s := range allShortcuts {
		if err := common.ValidateShortcut(s); err != nil {
			t.Errorf("invalid shortcut declaration: %v", err)
		}
	}
}

// TestNoLegacyMountFunctions AST-scans shortcuts/products and shortcuts/discounts
// for any func mount* declaration.
func TestNoLegacyMountFunctions(t *testing.T) {
	checkNoLegacyMounts(t)
}

func checkNoLegacyMounts(t *testing.T) {
	t.Helper()
	root := "."
	fset := token.NewFileSet()
	bad := []string{}
	err := filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return nil
		}
		// Only scan products/* and discounts/*.
		if !strings.Contains(path, "/products/") && !strings.Contains(path, "/discounts/") {
			return nil
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		for _, decl := range f.Decls {
			if fn, ok := decl.(*ast.FuncDecl); ok && strings.HasPrefix(fn.Name.Name, "mount") {
				bad = append(bad, path+": "+fn.Name.Name)
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	for _, b := range bad {
		t.Errorf("legacy mount function found (expected zero): %s", b)
	}
}
