package themes

import (
	"go/parser"
	"go/token"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

func TestImportsGuard_NoForbiddenPackages(t *testing.T) {
	forbidden := []string{
		"shoplazza-cli-v2/cmd",
		"shoplazza-cli-v2/internal/registry",
		"shoplazza-cli-v2/internal/serviceformat",
		// interactive deps — banned absolutely
		"github.com/AlecAivazis/survey",
		"github.com/manifoldco/promptui",
		"github.com/charmbracelet/bubbletea",
		"github.com/charmbracelet/huh",
		"golang.org/x/term",
	}

	_, thisFile, _, _ := runtime.Caller(0)
	themesDir := filepath.Dir(thisFile)                              // shortcuts/themes/
	repoRoot := filepath.Clean(filepath.Join(themesDir, "..", "..")) // repo root
	internalTheme := filepath.Join(repoRoot, "internal", "theme")

	// Walk both directories.
	for _, dir := range []string{themesDir, internalTheme} {
		walkAndCheckImports(t, dir, forbidden)
	}
}

func walkAndCheckImports(t *testing.T, dir string, forbidden []string) {
	t.Helper()
	_ = filepath.Walk(dir, func(p string, info os.FileInfo, err error) error {
		if err != nil || info.IsDir() {
			return nil
		}
		if !strings.HasSuffix(p, ".go") || strings.HasSuffix(p, "_test.go") {
			return nil
		}
		fset := token.NewFileSet()
		f, err := parser.ParseFile(fset, p, nil, parser.ImportsOnly)
		if err != nil {
			return nil
		}
		for _, imp := range f.Imports {
			path := strings.Trim(imp.Path.Value, `"`)
			for _, bad := range forbidden {
				if path == bad || strings.HasPrefix(path, bad+"/") {
					t.Errorf("%s: forbidden import %q", p, path)
				}
			}
		}
		return nil
	})
}
