package shortcuts

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

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
