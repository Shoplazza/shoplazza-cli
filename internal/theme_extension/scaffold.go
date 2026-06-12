package theme_extension

import (
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// Scaffold materializes the bundled basic|embed te template into destDir and
// applies v1's initProj rewrite. It keeps the theme-app/ wrapper (zip root +
// dev-doc type=parentDirName carrier) and writes nothing outside destDir. The
// extension.config.json is written by the caller via WriteConfig.
func Scaffold(destDir, name, themeType string) error {
	if themeType != "basic" && themeType != "embed" {
		return fmt.Errorf("invalid type %q: must be basic or embed", themeType)
	}
	sub, err := fs.Sub(templatesFS, "templates/"+themeType)
	if err != nil {
		return err
	}
	if err := copyEmbedTree(sub, destDir); err != nil {
		return err
	}

	app := filepath.Join(destDir, "theme-app")
	// rename block + snippet mains → <name> (v1 initProj filesToRename)
	if err := rename(filepath.Join(app, "blocks", "index.liquid"), filepath.Join(app, "blocks", name+".liquid")); err != nil {
		return err
	}
	if err := rename(filepath.Join(app, "snippets", "index.liquid"), filepath.Join(app, "snippets", name+".liquid")); err != nil {
		return err
	}
	// subtype-specific css rename
	if themeType == "basic" {
		if err := rename(filepath.Join(app, "assets", "index.css"), filepath.Join(app, "assets", name+".css")); err != nil {
			return err
		}
	} else {
		if err := rename(filepath.Join(app, "snippets", "index_css.liquid"), filepath.Join(app, "snippets", name+"_css.liquid")); err != nil {
			return err
		}
	}
	// replace {{projectName}} in package.json + the renamed block (v1 replacementConfig)
	for _, p := range []string{
		filepath.Join(destDir, "package.json"),
		filepath.Join(app, "blocks", name+".liquid"),
	} {
		if err := replaceInFile(p, "{{projectName}}", name); err != nil {
			return err
		}
	}
	return nil
}

// copyEmbedTree writes an embed.FS subtree to destDir.
func copyEmbedTree(src fs.FS, destDir string) error {
	return fs.WalkDir(src, ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		target := filepath.Join(destDir, filepath.FromSlash(p))
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := fs.ReadFile(src, p)
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, 0o644)
	})
}

func rename(oldPath, newPath string) error {
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func replaceInFile(path, oldS, newS string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, []byte(strings.ReplaceAll(string(data), oldS, newS)), 0o644)
}
