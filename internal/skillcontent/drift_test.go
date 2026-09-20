package skillcontent

import (
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strings"
	"testing"
)

// These guard tests convert "skill drift" from a silent runtime gap into a loud
// CI failure. They run under ./internal/... which CI executes, and they read the
// on-disk skills/ tree plus the root //go:embed directive directly — no binary,
// no network. The canonical rule: a skills/<dir>/ that contains a SKILL.md is a
// domain skill and MUST be embedded; anything without SKILL.md (the eval harness,
// _template) is tooling and MUST NOT be embedded.

// repoRoot walks up from the test's working directory to the module root.
func repoRoot(t *testing.T) string {
	t.Helper()
	dir, err := os.Getwd()
	if err != nil {
		t.Fatalf("getwd: %v", err)
	}
	for {
		if _, err := os.Stat(filepath.Join(dir, "go.mod")); err == nil {
			return dir
		}
		parent := filepath.Dir(dir)
		if parent == dir {
			t.Fatal("could not locate module root (no go.mod found walking up)")
		}
		dir = parent
	}
}

// onDiskDomains returns every skills/<dir>/ that contains a SKILL.md.
func onDiskDomains(t *testing.T, root string) []string {
	t.Helper()
	entries, err := os.ReadDir(filepath.Join(root, "skills"))
	if err != nil {
		t.Fatalf("read skills dir: %v", err)
	}
	var out []string
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		if _, err := os.Stat(filepath.Join(root, "skills", e.Name(), "SKILL.md")); err == nil {
			out = append(out, e.Name())
		}
	}
	sort.Strings(out)
	return out
}

// embeddedDomains parses the //go:embed directive(s) in skills_embed.go and
// returns the skill dir names they embed.
func embeddedDomains(t *testing.T, root string) []string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join(root, "skills_embed.go"))
	if err != nil {
		t.Fatalf("read skills_embed.go: %v", err)
	}
	var out []string
	for _, line := range strings.Split(string(data), "\n") {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "//go:embed ") {
			continue
		}
		for _, field := range strings.Fields(strings.TrimPrefix(line, "//go:embed ")) {
			field = strings.TrimPrefix(field, "all:")
			if !strings.HasPrefix(field, "skills/") {
				continue
			}
			// skills/<name>[/...] -> <name>
			rest := strings.TrimPrefix(field, "skills/")
			if i := strings.IndexByte(rest, '/'); i >= 0 {
				rest = rest[:i]
			}
			out = append(out, rest)
		}
	}
	sort.Strings(out)
	return out
}

func TestEveryDomainSkillIsEmbedded(t *testing.T) {
	root := repoRoot(t)
	disk := onDiskDomains(t, root)
	embedded := embeddedDomains(t, root)

	if strings.Join(disk, ",") != strings.Join(embedded, ",") {
		t.Errorf(`skill embed drift:
  on disk (skills/*/ with SKILL.md): %v
  in //go:embed (skills_embed.go):   %v
Fix: update the //go:embed directive in skills_embed.go so it lists exactly the
domain skills that have a SKILL.md.`, disk, embedded)
	}
	if len(disk) == 0 {
		t.Fatal("no domain skills found on disk — the layout or repoRoot detection is wrong")
	}
}

// TestSkillReferenceLinksResolve is a dead-link guard: every `references/*.md`
// a SKILL.md cites must exist on disk, so an embedded skill never points at a
// reference the binary doesn't carry.
func TestSkillReferenceLinksResolve(t *testing.T) {
	root := repoRoot(t)
	re := regexp.MustCompile(`references/[A-Za-z0-9._/-]+\.md`)
	for _, name := range onDiskDomains(t, root) {
		data, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if err != nil {
			t.Errorf("%s: read SKILL.md: %v", name, err)
			continue
		}
		seen := map[string]bool{}
		for _, rel := range re.FindAllString(string(data), -1) {
			if seen[rel] {
				continue
			}
			seen[rel] = true
			if _, err := os.Stat(filepath.Join(root, "skills", name, rel)); err != nil {
				t.Errorf("%s/SKILL.md cites %q but that file does not exist (dead reference link)", name, rel)
			}
		}
	}
}

func TestSkillFrontmatterNameMatchesDir(t *testing.T) {
	root := repoRoot(t)
	for _, name := range onDiskDomains(t, root) {
		data, err := os.ReadFile(filepath.Join(root, "skills", name, "SKILL.md"))
		if err != nil {
			t.Errorf("%s: read SKILL.md: %v", name, err)
			continue
		}
		if declared := frontmatterField(data, "name"); declared != name {
			t.Errorf("%s/SKILL.md declares name %q — it must match the directory name so 'skills read %s' resolves", name, declared, name)
		}
	}
}
