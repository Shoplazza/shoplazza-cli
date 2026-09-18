package theme

import (
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
)

// ReadInfo parses cwd/config/settings_schema.json to extract the theme name and
// version, with v1-compatible fallbacks:
//
//   - file missing            → validation error ("does not look like a Shoplazza theme")
//   - malformed JSON          → validation error ("config/settings_schema.json malformed: %v")
//   - other read failure      → internal error via ErrLocalIO
//   - theme_name missing      → filepath.Base(cwd)
//   - theme_version missing   → "unknown"
func ReadInfo(cwd string) (name, version string, err error) {
	p := filepath.Join(cwd, "config", "settings_schema.json")
	data, rerr := os.ReadFile(p)
	if errors.Is(rerr, fs.ErrNotExist) {
		return "", "", ErrNotThemeDir()
	}
	if rerr != nil {
		return "", "", ErrLocalIO("read settings_schema.json", rerr)
	}
	// config/settings_schema.json is a JSON ARRAY of setting groups; the
	// theme_info block is the element whose "name" is the string "theme_info",
	// with theme_name/theme_version as sibling keys. Elements are parsed as raw
	// field maps, not a typed struct, because other sections localize "name" as
	// an object ({"en":...,"zh":...}), which a []struct{Name string} could not
	// unmarshal.
	var arr []map[string]json.RawMessage
	if uerr := json.Unmarshal(data, &arr); uerr != nil {
		return "", "", ErrValidation("config/settings_schema.json malformed: %v", uerr)
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

// ZipName builds the "<name>-<version>.zip" artifact filename from theme
// metadata, sanitizing each component. The display name used elsewhere
// (progress lines, result bodies) stays untouched.
func ZipName(name, version string) string {
	return fmt.Sprintf("%s-%s.zip", SanitizeFileComponent(name), SanitizeFileComponent(version))
}

// sanitizeFileComponent makes a user-controlled string (theme_name /
// theme_version, theme ids) safe as a single filename component: path
// separators, ':' and control characters become '_'; values that reduce to
// "", "." or ".." degrade to "theme". This prevents a theme_name like
// "../../x" from escaping the target directory.
func SanitizeFileComponent(s string) string {
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
