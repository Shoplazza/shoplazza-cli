package common_test

import (
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"strings"
	"testing"
)

// Forbidden import prefixes that would break the shortcuts module boundary.
var forbiddenPrefixes = []string{
	"github.com/Shoplazza/shoplazza-cli/cmd/",
	"github.com/Shoplazza/shoplazza-cli/internal/registry",
	"github.com/Shoplazza/shoplazza-cli/internal/serviceformat",
}

func TestNoForbiddenImports(t *testing.T) {
	// Test runs with CWD == shortcuts/common; ".." resolves to the shortcuts/ root.
	root, err := filepath.Abs("..")
	if err != nil {
		t.Fatalf("abs ..: %v", err)
	}

	fset := token.NewFileSet()
	walked := 0
	err = filepath.WalkDir(root, func(path string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.IsDir() {
			return nil
		}
		if !strings.HasSuffix(path, ".go") {
			return nil
		}
		walked++
		f, err := parser.ParseFile(fset, path, nil, parser.ImportsOnly)
		if err != nil {
			return err
		}
		for _, imp := range f.Imports {
			val := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbiddenPrefixes {
				if strings.HasPrefix(val, bad) {
					pos := fset.Position(imp.Pos())
					rel, _ := filepath.Rel(root, path)
					t.Errorf("%s:%d: forbidden import %q (matches prefix %q)", rel, pos.Line, val, bad)
				}
			}
		}
		return nil
	})
	if err != nil {
		t.Fatalf("walk: %v", err)
	}
	if walked == 0 {
		t.Fatal("walked zero .go files under shortcuts/ — wrong root")
	}
}
