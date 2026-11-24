package shortcuts

import (
	"encoding/json"
	"fmt"
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"os"
	"path/filepath"
	"regexp"
	"slices"
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

// TestFlagEnumsMatchSpec checks every shortcut flag's Enum against the values
// the embedded API metadata documents for the parameter it fills, so a metadata
// sync that adds or drops a value fails here instead of in users' hands.
func TestFlagEnumsMatchSpec(t *testing.T) {
	type source struct{ leaf, param string }
	sources := map[string]source{
		"orders +search --status":             {"orders list", "status"},
		"orders +search --financial-status":   {"orders list", "financial_status"},
		"orders +search --fulfillment-status": {"orders list", "fulfillment_status"},
		"orders +count --status":              {"orders count", "status"},
		"orders +count --financial-status":    {"orders count", "financial_status"},
		"orders +count --fulfillment-status":  {"orders count", "fulfillment_status"},
		"discounts +search --progress":        {"discounts list", "progress"},
		"discounts +search --discount-type":   {"discounts list", "discount_type"},
		"discounts +search --discount-target": {"discounts list", "discount_targets"},
		"discounts +search --discount-method": {"discounts list", "discount_methods"},
	}
	checked := 0
	for _, s := range allShortcuts {
		for _, f := range s.Flags {
			if len(f.Enum) == 0 {
				continue
			}
			key := s.Service + " " + s.Command + " --" + f.Name
			src, ok := sources[key]
			if !ok {
				t.Errorf("%s has an Enum but no metadata parameter to check it against; add it to sources", key)
				continue
			}
			want := specEnum(t, src.leaf, src.param)
			var missing, extra []string
			for _, v := range want {
				if !slices.Contains(f.Enum, v) {
					missing = append(missing, v)
				}
			}
			for _, v := range f.Enum {
				if !slices.Contains(want, v) {
					extra = append(extra, v)
				}
			}
			if len(missing) > 0 || len(extra) > 0 {
				t.Errorf("%s Enum drifted from %s %s: add %v, remove %v", key, src.leaf, src.param, missing, extra)
			}
			checked++
		}
	}
	if checked != len(sources) {
		t.Errorf("checked %d Enum flags, but sources lists %d; drop entries whose flag has no Enum", checked, len(sources))
	}
}

// enumItem matches a "- value:" or "- `value`:" entry of a parameter description.
var enumItem = regexp.MustCompile("(?:^|\\s)- `?([a-z][a-z0-9_]*)`?:")

// specEnum returns the values the embedded API metadata documents for param of
// the "<module> <command path>" leaf: its enum, else the "- value:" entries of
// its description. The file is read directly: shortcuts/ must not import
// internal/registry (TestNoForbiddenImports).
func specEnum(t *testing.T, leaf, param string) []string {
	t.Helper()
	raw, err := os.ReadFile("../internal/registry/cli_meta_v202601_en.json")
	if err != nil {
		t.Fatalf("embedded metadata: %v", err)
	}
	type schemaParam struct {
		Name        string `json:"name"`
		Description string `json:"description"`
		Enum        []any  `json:"enum"`
		Items       *struct {
			Enum []any `json:"enum"`
		} `json:"items"`
	}
	var meta struct {
		Modules []struct {
			Name     string `json:"name"`
			Commands []struct {
				Path       []string      `json:"command_path"`
				Parameters []schemaParam `json:"parameters"`
			} `json:"commands"`
		} `json:"modules"`
	}
	if err := json.Unmarshal(raw, &meta); err != nil {
		t.Fatalf("embedded metadata: %v", err)
	}
	for _, m := range meta.Modules {
		for _, c := range m.Commands {
			if m.Name+" "+strings.Join(c.Path, " ") != leaf {
				continue
			}
			for _, p := range c.Parameters {
				if p.Name != param {
					continue
				}
				enum := p.Enum
				if len(enum) == 0 && p.Items != nil {
					enum = p.Items.Enum
				}
				var values []string
				for _, v := range enum {
					values = append(values, fmt.Sprint(v))
				}
				if len(values) == 0 {
					for _, m := range enumItem.FindAllStringSubmatch(p.Description, -1) {
						values = append(values, m[1])
					}
				}
				if len(values) == 0 {
					t.Fatalf("%s: parameter %s lists no values in the metadata", leaf, param)
				}
				return values
			}
			t.Fatalf("%s has no parameter %s in the metadata", leaf, param)
		}
	}
	t.Fatalf("no %q leaf in the metadata", leaf)
	return nil
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
