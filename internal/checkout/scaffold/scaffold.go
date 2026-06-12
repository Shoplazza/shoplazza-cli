// Package scaffold writes new checkout extension projects from an embedded
// template. The template intentionally omits extension.config.js:
// store/token come from global `shoplazza auth`, not a repo file.
package scaffold

import (
	"embed"
	"encoding/json"
	"io/fs"
	"os"
	"path/filepath"
)

//go:embed all:template
var templateFS embed.FS

const (
	templateRoot = "template"
	extTemplate  = "template/extensions/extension-template"
)

// Project scaffolds a new project at destDir (which must not exist), with one
// initial extension under destDir/extensions/<firstExt>. On a partial failure
// the dir is removed again only if Project created it; a pre-existing destDir is
// never deleted.
func Project(destDir, projectName, firstExt string) error {
	created := false
	if _, statErr := os.Stat(destDir); os.IsNotExist(statErr) {
		created = true
	}
	err := scaffoldProject(destDir, projectName, firstExt)
	if err != nil && created {
		_ = os.RemoveAll(destDir) // best-effort: never leave a half-written project
	}
	return err
}

func scaffoldProject(destDir, projectName, firstExt string) error {
	if err := os.MkdirAll(destDir, 0o755); err != nil {
		return err
	}
	entries, err := templateFS.ReadDir(templateRoot)
	if err != nil {
		return err
	}
	for _, e := range entries {
		if e.IsDir() {
			continue // the only dir is extensions/, handled below
		}
		data, rErr := templateFS.ReadFile(templateRoot + "/" + e.Name())
		if rErr != nil {
			return rErr
		}
		name := e.Name()
		if name == "_gitignore" {
			name = ".gitignore"
		}
		if name == "package.json" {
			if data, err = setJSONField(data, "name", projectName); err != nil {
				return err
			}
		}
		if err = os.WriteFile(filepath.Join(destDir, name), data, 0o644); err != nil {
			return err
		}
	}
	return Extension(destDir, firstExt)
}

// Extension scaffolds one extension under projectRoot/extensions/<extName>,
// filling extensionName (extensionId stays empty until first push). Like
// Project, a partial failure removes the target dir only if Extension
// created it.
func Extension(projectRoot, extName string) error {
	dstExtDir := filepath.Join(projectRoot, "extensions", extName)
	created := false
	if _, statErr := os.Stat(dstExtDir); os.IsNotExist(statErr) {
		created = true
	}
	err := fs.WalkDir(templateFS, extTemplate, func(p string, d fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		rel, relErr := filepath.Rel(extTemplate, p)
		if relErr != nil {
			return relErr
		}
		target := filepath.Join(dstExtDir, rel)
		if d.IsDir() {
			return os.MkdirAll(target, 0o755)
		}
		data, err := templateFS.ReadFile(p)
		if err != nil {
			return err
		}
		if filepath.Base(p) == "extension.json" {
			if data, err = setJSONField(data, "extensionName", extName); err != nil {
				return err
			}
		}
		return os.WriteFile(target, data, 0o644)
	})
	if err != nil && created {
		_ = os.RemoveAll(dstExtDir) // best-effort: never leave a half-written extension
	}
	return err
}

func setJSONField(data []byte, key, value string) ([]byte, error) {
	var m map[string]any
	if err := json.Unmarshal(data, &m); err != nil {
		return nil, err
	}
	m[key] = value
	out, err := json.MarshalIndent(m, "", "  ")
	if err != nil {
		return nil, err
	}
	return append(out, '\n'), nil
}
