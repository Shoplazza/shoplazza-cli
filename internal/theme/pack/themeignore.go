package pack

import (
	"errors"
	"io/fs"
	"os"
	"path/filepath"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Ignorer matches relative (forward-slash) paths against compiled .themeignore
// rules, hiding the third-party gitignore type from callers.
type Ignorer interface {
	MatchesPath(rel string) bool
}

// LoadThemeIgnorer compiles the .themeignore matcher with the same
// PackOptions.IgnoreFile semantics Pack uses ("" → auto-detect at srcDir,
// "/dev/null" → force-disable, explicit path → that file). Returns a nil
// Ignorer when no ignore file applies — callers must nil-check.
func LoadThemeIgnorer(srcDir, explicitPath string) (Ignorer, error) {
	gi, err := loadIgnorer(srcDir, explicitPath)
	if err != nil {
		return nil, err
	}
	if gi == nil {
		return nil, nil
	}
	return gi, nil
}

// loadIgnorer returns a compiled .themeignore matcher per PackOptions.IgnoreFile semantics.
// Returns nil (no filtering) when no ignore file should apply.
func loadIgnorer(srcDir, explicitPath string) (*gitignore.GitIgnore, error) {
	path := explicitPath
	switch path {
	case "":
		// auto-detect
		path = filepath.Join(srcDir, ".themeignore")
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			return nil, nil
		}
	case "/dev/null":
		// sentinel for --no-ignore
		return nil, nil
	default:
		if _, err := os.Stat(path); errors.Is(err, fs.ErrNotExist) {
			// explicit path missing → fail silently (force disable)
			return nil, nil
		}
	}
	gi, err := gitignore.CompileIgnoreFile(path)
	if err != nil {
		return nil, err
	}
	return gi, nil
}
