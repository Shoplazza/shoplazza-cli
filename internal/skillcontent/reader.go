// Package skillcontent serves the Agent Skills embedded in the CLI binary at
// build time, so an agent can read the guidance that matches the exact binary
// it is running — no filesystem lookup, no version skew with ~/.agents/skills.
package skillcontent

import (
	"fmt"
	"io/fs"
	"path"
	"sort"
	"strings"
)

// embedded is the skill-content FS (each <name>/SKILL.md + <name>/references/*)
// injected by main via SetFS. Nil until set — e.g. in unit tests that don't
// wire the embed, where a Reader is constructed with a test FS instead.
var embedded fs.FS

// SetFS registers the embedded skill FS. Call once, from main, before Execute.
func SetFS(fsys fs.FS) { embedded = fsys }

// Embedded returns the registered skill FS (nil if none).
func Embedded() fs.FS { return embedded }

// Reader reads skill content from an fs.FS rooted at the skills directory, i.e.
// its top-level entries are skill dirs like "shoplazza-orders".
type Reader struct{ fsys fs.FS }

// New returns a Reader over fsys.
func New(fsys fs.FS) *Reader { return &Reader{fsys: fsys} }

// SkillInfo is one skill's identity for `skills list`.
type SkillInfo struct {
	Name        string `json:"name"`
	Description string `json:"description,omitempty"`
}

// List returns every embedded skill (a top-level dir containing SKILL.md),
// with its name and frontmatter description, sorted by name.
func (r *Reader) List() ([]SkillInfo, error) {
	if r.fsys == nil {
		return nil, nil
	}
	entries, err := fs.ReadDir(r.fsys, ".")
	if err != nil {
		return nil, err
	}
	out := make([]SkillInfo, 0, len(entries))
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		data, err := fs.ReadFile(r.fsys, path.Join(e.Name(), "SKILL.md"))
		if err != nil {
			continue // a dir without SKILL.md isn't a skill
		}
		out = append(out, SkillInfo{Name: e.Name(), Description: frontmatterDescription(data)})
	}
	sort.Slice(out, func(i, j int) bool { return out[i].Name < out[j].Name })
	return out, nil
}

// ReadSkill returns a skill's SKILL.md bytes.
func (r *Reader) ReadSkill(name string) ([]byte, error) {
	if err := r.ensure(); err != nil {
		return nil, err
	}
	if err := guardName(name); err != nil {
		return nil, err
	}
	return fs.ReadFile(r.fsys, path.Join(name, "SKILL.md"))
}

// ReadReference returns a file under a skill (e.g. references/refunds.md),
// plus the cleaned relative path actually read.
func (r *Reader) ReadReference(name, relpath string) ([]byte, string, error) {
	if err := r.ensure(); err != nil {
		return nil, "", err
	}
	if err := guardName(name); err != nil {
		return nil, "", err
	}
	clean, err := guardRel(relpath)
	if err != nil {
		return nil, "", err
	}
	b, err := fs.ReadFile(r.fsys, path.Join(name, clean))
	return b, clean, err
}

// ListPath lists one layer under name[/subpath] like `ls`; dirs end with "/".
func (r *Reader) ListPath(name, subpath string) ([]string, string, error) {
	if err := r.ensure(); err != nil {
		return nil, "", err
	}
	if err := guardName(name); err != nil {
		return nil, "", err
	}
	clean := "."
	if subpath != "" {
		var err error
		if clean, err = guardRel(subpath); err != nil {
			return nil, "", err
		}
	}
	dir := path.Join(name, clean)
	entries, err := fs.ReadDir(r.fsys, dir)
	if err != nil {
		return nil, "", err
	}
	names := make([]string, 0, len(entries))
	for _, e := range entries {
		n := e.Name()
		if e.IsDir() {
			n += "/"
		}
		names = append(names, n)
	}
	sort.Strings(names)
	return names, dir, nil
}

func (r *Reader) ensure() error {
	if r.fsys == nil {
		return fmt.Errorf("no skills are embedded in this binary")
	}
	return nil
}

// SplitArg maps "name" or "name/rel/path" to (name, relpath) by the first slash.
func SplitArg(arg string) (name, relpath string) {
	if i := strings.IndexByte(arg, '/'); i >= 0 {
		return arg[:i], arg[i+1:]
	}
	return arg, ""
}

// guardName rejects a skill name that isn't a single safe path segment.
func guardName(name string) error {
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return fmt.Errorf("invalid skill name %q", name)
	}
	return nil
}

// guardRel rejects any relative path that would escape the skill directory.
func guardRel(rel string) (string, error) {
	clean := path.Clean(rel)
	if clean == "." || clean == ".." || strings.HasPrefix(clean, "../") || strings.HasPrefix(clean, "/") {
		return "", fmt.Errorf("invalid path %q (must stay within the skill)", rel)
	}
	return clean, nil
}

// frontmatterField returns a single-line frontmatter value (e.g. `name`),
// trimmed and unquoted. Returns "" when absent or not within a frontmatter
// block. Folded/multi-line values are not handled — use frontmatterDescription.
func frontmatterField(data []byte, key string) string {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return ""
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return ""
	}
	prefix := key + ":"
	for _, ln := range strings.Split(s[3:3+end], "\n") {
		if strings.HasPrefix(ln, prefix) {
			return strings.Trim(strings.TrimSpace(strings.TrimPrefix(ln, prefix)), `"'`)
		}
	}
	return ""
}

// frontmatterDescription extracts the `description` field from a SKILL.md YAML
// frontmatter block, joining a folded (>-) multi-line value into one line.
// Returns "" when there's no frontmatter or no description. Deliberately a tiny
// hand parser — the frontmatter here only ever carries name + description.
func frontmatterDescription(data []byte) string {
	s := string(data)
	if !strings.HasPrefix(s, "---") {
		return ""
	}
	end := strings.Index(s[3:], "\n---")
	if end < 0 {
		return ""
	}
	lines := strings.Split(s[3:3+end], "\n")
	var desc []string
	capturing := false
	for _, ln := range lines {
		if capturing {
			// A continuation is indented; a new unindented "key:" ends the block.
			if len(ln) > 0 && ln[0] != ' ' && ln[0] != '\t' && strings.Contains(ln, ":") {
				break
			}
			if t := strings.TrimSpace(ln); t != "" {
				desc = append(desc, t)
			}
			continue
		}
		if strings.HasPrefix(ln, "description:") {
			v := strings.TrimSpace(strings.TrimPrefix(ln, "description:"))
			if v == "" || v == ">" || v == ">-" || v == "|" || v == "|-" {
				capturing = true
				continue
			}
			return strings.Trim(v, `"'`)
		}
	}
	return strings.TrimSpace(strings.Join(desc, " "))
}
