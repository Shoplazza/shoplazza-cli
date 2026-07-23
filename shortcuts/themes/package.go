package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

// packageShortcut is the `themes package` workflow: reads
// config/settings_schema.json from the cwd to derive <theme_name,
// theme_version>, then zips the cwd into "<name>-<version>.zip" via
// internal/theme/pack.Pack. The operation is entirely local.
//
// .themeignore handling is delegated to pack.PackOptions: the default
// auto-detects .themeignore at srcDir, while --no-ignore force-disables it.
// Filename fallbacks (mirroring v1) are filepath.Base(cwd) for a missing
// theme_name and "unknown" for a missing theme_version. A missing or
// malformed config/settings_schema.json is a validation error.
var packageShortcut = common.Shortcut{
	Service: "themes",
	Command: "package",
	Use:     "package",
	Short:   "Package the current theme directory into a zip",
	// Purely local (reads cwd, writes a zip): runs without login and reports a
	// local artifact rather than an API response (no {ok,data} envelope).
	// Writes the local filesystem, so blind scans skip it.
	AuthFree:     true,
	Local:        true,
	NotScannable: true,
	Flags: []common.Flag{
		{
			Name:        "no-ignore",
			Type:        common.FlagBool,
			Default:     false,
			Description: "Ignore .themeignore",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		noIgnore := in.Flags.GetBool("no-ignore")
		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}
		name, version, err := readThemeInfo(cwd)
		if err != nil {
			return common.ExecResult{}, err
		}
		zipName := themeZipName(name, version)
		out := filepath.Join(cwd, zipName)
		// ExecInput has no Stderr field yet, so write directly to os.Stderr.
		fmt.Fprintf(os.Stderr, "[package] packaging into %s\n", zipName)

		opts := pack.PackOptions{}
		if noIgnore {
			// "/dev/null" is pack.PackOptions' sentinel for force-disable.
			opts.IgnoreFile = "/dev/null"
		}

		if in.DryRun {
			// Dry-run: skip pack.Pack (no disk side-effects) but still
			// enumerate for a file_count. A silent file_count of 0 previously
			// masked permission errors, so surface enumeration failures.
			files, eerr := pack.EnumerateThemeFiles(cwd)
			if eerr != nil {
				return common.ExecResult{}, theme.ErrLocalIO("enumerate theme files", eerr)
			}
			return common.ExecResult{Body: map[string]any{
				"zip_path":   out,
				"file_count": len(files),
				"dry_run":    true,
				"name":       name,
				"version":    version,
				"no_ignore":  noIgnore,
			}}, nil
		}

		zipPath, err := pack.Pack(cwd, out, opts)
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("pack zip", err)
		}
		return common.ExecResult{Body: map[string]any{
			"zip_path": zipPath,
			"name":     name,
			"version":  version,
		}}, nil
	},
}

// themeZipName builds the "<name>-<version>.zip" artifact filename from theme
// metadata, sanitizing each component. The display name used elsewhere
// (progress lines, result bodies) stays untouched.
func themeZipName(name, version string) string {
	return fmt.Sprintf("%s-%s.zip", sanitizeFileComponent(name), sanitizeFileComponent(version))
}

// sanitizeFileComponent makes a user-controlled string (theme_name /
// theme_version, theme ids) safe as a single filename component: path
// separators, ':' and control characters become '_'; values that reduce to
// "", "." or ".." degrade to "theme". This prevents a theme_name like
// "../../x" from escaping the target directory.
func sanitizeFileComponent(s string) string {
	var b strings.Builder
	for _, r := range s {
		if r == '/' || r == '\\' || r == ':' || r < 0x20 || r == 0x7f {
			b.WriteRune('_')
			continue
		}
		b.WriteRune(r)
	}
	out := strings.TrimSpace(b.String())
	if out == "" || out == "." || out == ".." {
		return "theme"
	}
	return out
}

// readThemeInfo parses cwd/config/settings_schema.json to extract the theme
// name and version, with v1-compatible fallbacks:
//
//   - file missing            → validation error ("does not look like a Shoplazza theme")
//   - malformed JSON          → validation error ("config/settings_schema.json malformed: %v")
//   - other read failure      → internal error via theme.ErrLocalIO
//   - theme_name missing      → filepath.Base(cwd)
//   - theme_version missing   → "unknown"
func readThemeInfo(cwd string) (name, version string, err error) {
	p := filepath.Join(cwd, "config", "settings_schema.json")
	data, rerr := os.ReadFile(p)
	if errors.Is(rerr, fs.ErrNotExist) {
		return "", "", theme.ErrValidation(
			"this directory does not look like a Shoplazza theme (config/settings_schema.json missing)")
	}
	if rerr != nil {
		return "", "", theme.ErrLocalIO("read settings_schema.json", rerr)
	}
	// config/settings_schema.json is a JSON ARRAY of setting groups; the
	// theme_info block is the element whose "name" is the string "theme_info",
	// with theme_name/theme_version as sibling keys. Elements are parsed as raw
	// field maps, not a typed struct, because other sections localize "name" as
	// an object ({"en":...,"zh":...}), which a []struct{Name string} could not
	// unmarshal.
	var arr []map[string]json.RawMessage
	if uerr := json.Unmarshal(data, &arr); uerr != nil {
		return "", "", theme.ErrValidation("config/settings_schema.json malformed: %v", uerr)
	}
	for _, el := range arr {
		var n string
		if json.Unmarshal(el["name"], &n) == nil && n == "theme_info" {
			_ = json.Unmarshal(el["theme_name"], &name) // leave "" if absent/non-string
			_ = json.Unmarshal(el["theme_version"], &version)
			break
		}
	}
	if name == "" {
		name = filepath.Base(cwd)
	}
	if version == "" {
		version = "unknown"
	}
	return name, version, nil
}
