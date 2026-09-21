// Package env parses the project-level theme environment file
// (shoplazza.theme.toml) and resolves a named environment to the values that
// seed theme-command flags. It is the foundation of theme multi-environment
// ("one config, many stores"): pure parsing + find-up + lookup, with no
// terminal, network, or flag wiring — those live at the injection site.
//
// The file is git-committed and carries NO secrets: an environment names a
// store / theme / path / ignore rules and, at most, a profile to authenticate
// with. Credentials always come from the keychain profile or the CI env token,
// never from this file (safer than Shopify, which allows an inline token).
package env

import (
	"bytes"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"sort"

	"github.com/BurntSushi/toml"
)

// FileName is the project-level theme environment file, discovered by walking up
// from the working directory (mirrors Shopify's shopify.theme.toml).
const FileName = "shoplazza.theme.toml"

// DefaultEnvironment is the environment applied when none is named on the
// command line (optional; absent → today's behavior is unchanged).
const DefaultEnvironment = "default"

// EnvironmentFlag / EnvironmentVar name the -e selector the theme commands
// expose and its CI env-var equivalent. They live here (not in cmdutil) so the
// multi-environment concept stays entirely in theme-owned packages.
const (
	EnvironmentFlag = "environment"
	EnvironmentVar  = "SHOPLAZZA_CLI_ENVIRONMENT"
)

// File is a parsed shoplazza.theme.toml. Unknown top-level keys are ignored so
// the format can grow without breaking older CLIs.
type File struct {
	Version      int                    `toml:"version"`
	Environments map[string]Environment `toml:"environments"`
}

// Environment is one [environments.<name>] block. Its keys mirror theme-command
// inputs; only the ones a given command actually accepts take effect at the
// injection site. Zero values mean "unset" and never override an inferred value.
type Environment struct {
	// Store is the target store domain (e.g. my-dev.myshoplaza.com).
	Store string `toml:"store"`
	// Theme is the target theme id; seeds --theme-id.
	Theme string `toml:"theme"`
	// Path is the theme project root; seeds --path where the command has one.
	Path string `toml:"path"`
	// Ignore lists glob patterns to skip; seeds --ignore where present.
	Ignore []string `toml:"ignore"`
	// Profile names the keychain profile to authenticate with. Empty → the
	// store is matched against the profile library (today's behavior).
	Profile string `toml:"profile"`
	// Config optionally activates a sibling app config (shoplazza.app.<config>.toml)
	// for projects that hold both an app and a theme.
	Config string `toml:"config"`
}

// ErrNotFound reports that no shoplazza.theme.toml exists at or above the start
// directory. Callers treat it as "no environments configured" (today's
// behavior), not as a hard error.
var ErrNotFound = errors.New("no " + FileName + " found")

// Find walks up from startDir (inclusive) to the filesystem root looking for
// FileName, returning its absolute path. It returns ErrNotFound when none
// exists — the signal to fall back to today's single-store behavior.
func Find(startDir string) (string, error) {
	dir, err := filepath.Abs(startDir)
	if err != nil {
		return "", err
	}
	for {
		candidate := filepath.Join(dir, FileName)
		if info, statErr := os.Stat(candidate); statErr == nil && !info.IsDir() {
			return candidate, nil
		}
		parent := filepath.Dir(dir)
		if parent == dir { // reached the filesystem root
			return "", ErrNotFound
		}
		dir = parent
	}
}

// Load parses the file at path. A decode error is wrapped with the path so the
// caller can surface a precise, actionable message.
func Load(path string) (File, error) {
	var f File
	if _, err := toml.DecodeFile(path, &f); err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return File{}, ErrNotFound
		}
		return File{}, fmt.Errorf("parse %s: %w", path, err)
	}
	return f, nil
}

// Environment returns the block named name. An empty name resolves to
// DefaultEnvironment. The bool is false when the environment is absent, letting
// the caller distinguish "no such environment" (a user error worth a structured
// message) from a present-but-empty block.
func (f File) Environment(name string) (Environment, bool) {
	if name == "" {
		name = DefaultEnvironment
	}
	env, ok := f.Environments[name]
	return env, ok
}

// Save writes f to path as TOML. It re-encodes from the model — every
// environment renders only the fields it actually sets (empty strings, empty
// ignore lists and live=false are omitted), so blocks stay clean. Comments and
// hand-authored formatting are NOT preserved: `env add/set/remove` are helpers;
// hand-editing shoplazza.theme.toml remains fully supported.
func Save(path string, f File) error {
	root := map[string]any{}
	if f.Version != 0 {
		root["version"] = f.Version
	}
	envs := map[string]any{}
	for name, e := range f.Environments {
		m := map[string]any{}
		if e.Store != "" {
			m["store"] = e.Store
		}
		if e.Theme != "" {
			m["theme"] = e.Theme
		}
		if e.Path != "" {
			m["path"] = e.Path
		}
		if len(e.Ignore) > 0 {
			m["ignore"] = e.Ignore
		}
		if e.Profile != "" {
			m["profile"] = e.Profile
		}
		if e.Config != "" {
			m["config"] = e.Config
		}
		envs[name] = m
	}
	root["environments"] = envs

	var buf bytes.Buffer
	if err := toml.NewEncoder(&buf).Encode(root); err != nil {
		return fmt.Errorf("encode %s: %w", FileName, err)
	}
	return os.WriteFile(path, buf.Bytes(), 0o644)
}

// Names returns the defined environment names in sorted order, for `env list`
// and for a "did you mean" hint when a name is missing.
func (f File) Names() []string {
	names := make([]string, 0, len(f.Environments))
	for name := range f.Environments {
		names = append(names, name)
	}
	sort.Strings(names)
	return names
}
