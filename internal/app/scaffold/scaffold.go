// Package scaffold rewrites an already-cloned extension template into a finished
// extension. It performs no git and no network I/O, so the rewrite is
// unit-testable in isolation.
//
// The theme rewrite applies steps 2–9 of the theme scaffold rule; clone and
// placement are the caller's responsibility, with this package handling the
// src→dest copy. checkout/function extensions use the simpler rewrite
// (package.json name + shoplazza.extension.toml {name,type}, no subtype branch).
package scaffold

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"

	"github.com/BurntSushi/toml"
)

const extTOML = "shoplazza.extension.toml"

// ScaffoldTheme rewrites the already-cloned theme template at srcDir into a
// finished theme extension at destDir, applying the basic/embed rewrite.
// themeType must be "basic" or "embed".
func ScaffoldTheme(srcDir, destDir, name, themeType string) error {
	typeValue, err := themeTypeValue(themeType)
	if err != nil {
		return err
	}
	if err := copyTree(srcDir, destDir); err != nil {
		return err
	}

	// step 2: package.json name.
	if err := setJSONField(filepath.Join(destDir, "package.json"), "name", name); err != nil {
		return err
	}
	// step 3: shoplazza.extension.toml name + type=theme.
	if err := setTOMLFields(filepath.Join(destDir, extTOML), name, "theme"); err != nil {
		return err
	}

	// step 4: block main file index-<themeType>.liquid → <name>.liquid.
	blockMain := filepath.Join(destDir, "blocks", name+".liquid")
	if err := rename(filepath.Join(destDir, "blocks", "index-"+themeType+".liquid"), blockMain); err != nil {
		return err
	}
	// step 5: snippet main file index.liquid → <name>.liquid.
	if err := rename(filepath.Join(destDir, "snippets", "index.liquid"),
		filepath.Join(destDir, "snippets", name+".liquid")); err != nil {
		return err
	}
	// step 6: subtype branch.
	cssSnippet := filepath.Join(destDir, "snippets", "index_css.liquid")
	switch themeType {
	case "embed":
		if err := rename(cssSnippet, filepath.Join(destDir, "snippets", name+"_css.liquid")); err != nil {
			return err
		}
		if err := os.RemoveAll(filepath.Join(destDir, "assets")); err != nil {
			return err
		}
	case "basic":
		if err := os.Remove(cssSnippet); err != nil {
			return err
		}
		if err := rename(filepath.Join(destDir, "assets", "index.css"),
			filepath.Join(destDir, "assets", name+".css")); err != nil {
			return err
		}
	}
	// step 7: replace {{projectName}} in the renamed block.
	if err := replaceInFile(blockMain, "{{projectName}}", name); err != nil {
		return err
	}
	// step 8: replace {{type}} in both locales.
	for _, loc := range []string{"en-US.json", "zh-CN.json"} {
		if err := replaceInFile(filepath.Join(destDir, "locales", loc), "{{type}}", typeValue); err != nil {
			return err
		}
	}
	// step 9: delete the other subtype's block template.
	other := "basic"
	if themeType == "basic" {
		other = "embed"
	}
	return os.Remove(filepath.Join(destDir, "blocks", "index-"+other+".liquid"))
}

// ScaffoldSimple rewrites the already-cloned template at srcDir into a finished
// checkout/function extension at destDir: copy + package.json name +
// shoplazza.extension.toml {name, type=extType}. No liquid/locale rewrite.
func ScaffoldSimple(srcDir, destDir, name, extType string) error {
	if err := copyTree(srcDir, destDir); err != nil {
		return err
	}
	if err := setJSONField(filepath.Join(destDir, "package.json"), "name", name); err != nil {
		return err
	}
	return setTOMLFields(filepath.Join(destDir, extTOML), name, extType)
}

func themeTypeValue(themeType string) (string, error) {
	switch themeType {
	case "basic":
		return "BASIC", nil
	case "embed":
		return "EMBED", nil
	default:
		return "", fmt.Errorf("invalid theme-type %q: must be basic or embed", themeType)
	}
}

// copyTree copies the srcDir tree to destDir, skipping any .git directory and
// preserving file modes. destDir's parent is created as needed.
func copyTree(srcDir, destDir string) error {
	return fs.WalkDir(os.DirFS(srcDir), ".", func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if d.Name() == ".git" {
			if d.IsDir() {
				return fs.SkipDir
			}
			return nil
		}
		target := filepath.Join(destDir, filepath.FromSlash(p))
		if d.IsDir() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			return os.MkdirAll(target, info.Mode().Perm()|0o700)
		}
		info, err := d.Info()
		if err != nil {
			return err
		}
		data, err := os.ReadFile(filepath.Join(srcDir, filepath.FromSlash(p)))
		if err != nil {
			return err
		}
		if err := os.MkdirAll(filepath.Dir(target), 0o755); err != nil {
			return err
		}
		return os.WriteFile(target, data, info.Mode().Perm())
	})
}

func rename(oldPath, newPath string) error {
	if err := os.MkdirAll(filepath.Dir(newPath), 0o755); err != nil {
		return err
	}
	return os.Rename(oldPath, newPath)
}

func replaceInFile(path, old, new string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return os.WriteFile(path, bytes.ReplaceAll(data, []byte(old), []byte(new)), 0o644)
}

func setJSONField(path, key, value string) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return err
	}
	m[key] = value
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(path, append(out, '\n'), 0o644)
}

// setTOMLFields decodes the toml at path into a map, sets name/type, and
// re-encodes. Exact key order is not preserved (not required for a scaffold).
func setTOMLFields(path, name, typ string) error {
	var m map[string]any
	if _, err := toml.DecodeFile(path, &m); err != nil {
		return err
	}
	if m == nil {
		m = map[string]any{}
	}
	m["name"] = name
	m["type"] = typ
	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(m); err != nil {
		return err
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}
