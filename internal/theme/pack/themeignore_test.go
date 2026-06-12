package pack

import (
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"
)

func writeThemeignore(t *testing.T, dir, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(dir, ".themeignore"), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}

func TestThemeignore_BasicGlob(t *testing.T) {
	root := setupThemeDir(t)
	writeThemeignore(t, root, "*.bak\n")
	_ = os.WriteFile(filepath.Join(root, "assets", "main.css.bak"), []byte("x"), 0o644)
	out := filepath.Join(root, "test.zip")
	if _, err := Pack(root, out, PackOptions{}); err != nil {
		t.Fatal(err)
	}
	for _, e := range zipEntries(t, out) {
		if e == "assets/main.css.bak" {
			t.Fatalf(".bak file should be excluded: %v", zipEntries(t, out))
		}
	}
}

func TestThemeignore_DoubleStarRecursive(t *testing.T) {
	root := setupThemeDir(t)
	_ = os.WriteFile(filepath.Join(root, "assets", "draft-x.css"), []byte("x"), 0o644)
	_ = os.MkdirAll(filepath.Join(root, "assets", "sub"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "assets", "sub", "draft-y.css"), []byte("y"), 0o644)
	writeThemeignore(t, root, "**/draft-*.css\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	for _, e := range zipEntries(t, out) {
		if e == "assets/draft-x.css" || e == "assets/sub/draft-y.css" {
			t.Fatalf("draft-*.css should be excluded recursively: %v", zipEntries(t, out))
		}
	}
}

func TestThemeignore_DirectoryTrailingSlash(t *testing.T) {
	root := setupThemeDir(t)
	_ = os.MkdirAll(filepath.Join(root, "node_modules", "foo"), 0o755)
	_ = os.WriteFile(filepath.Join(root, "node_modules", "foo", "bar.js"), []byte("x"), 0o644)
	writeThemeignore(t, root, "node_modules/\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	for _, e := range zipEntries(t, out) {
		if e == "node_modules/foo/bar.js" {
			t.Fatalf("node_modules/ should be fully excluded: %v", zipEntries(t, out))
		}
	}
}

func TestThemeignore_NegationRescuesFile(t *testing.T) {
	root := setupThemeDir(t)
	_ = os.WriteFile(filepath.Join(root, "assets", "foo.css"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "assets", "bar.css"), []byte("x"), 0o644)
	writeThemeignore(t, root, "assets/*.css\n!assets/main.css\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	entries := zipEntries(t, out)
	hasMain, hasFoo := false, false
	for _, e := range entries {
		if e == "assets/main.css" {
			hasMain = true
		}
		if e == "assets/foo.css" {
			hasFoo = true
		}
	}
	if !hasMain {
		t.Errorf("assets/main.css should be rescued by negation")
	}
	if hasFoo {
		t.Errorf("assets/foo.css should remain excluded")
	}
}

func TestThemeignore_AnchoredRoot(t *testing.T) {
	root := setupThemeDir(t)
	_ = os.WriteFile(filepath.Join(root, "draft.txt"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "assets", "draft.txt"), []byte("x"), 0o644)
	writeThemeignore(t, root, "/draft.txt\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	entries := zipEntries(t, out)
	for _, e := range entries {
		if e == "draft.txt" {
			t.Errorf("/draft.txt should be excluded at root")
		}
	}
	// assets/draft.txt is not in any standard theme dir for write, but stored as assets/draft.txt
	// is under "assets" which IS a theme dir, so it ships unless ignored.
	found := false
	for _, e := range entries {
		if e == "assets/draft.txt" {
			found = true
		}
	}
	if !found {
		t.Errorf("assets/draft.txt should not be matched by /draft.txt")
	}
}

func TestThemeignore_CommentsSkipped(t *testing.T) {
	root := setupThemeDir(t)
	writeThemeignore(t, root, "# this is a comment\n*.draft\n")
	_ = os.WriteFile(filepath.Join(root, "assets", "x.draft"), []byte("x"), 0o644)
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	for _, e := range zipEntries(t, out) {
		if e == "assets/x.draft" {
			t.Fatalf("*.draft should be excluded; comment should not interfere")
		}
	}
}

func TestThemeignore_LastMatchWins(t *testing.T) {
	root := setupThemeDir(t)
	_ = os.WriteFile(filepath.Join(root, "assets", "secret.css"), []byte("x"), 0o644)
	_ = os.WriteFile(filepath.Join(root, "assets", "main.css"), []byte("x"), 0o644)
	writeThemeignore(t, root, "*.css\n!assets/*.css\nassets/secret.css\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	entries := zipEntries(t, out)
	hasMain, hasSecret := false, false
	for _, e := range entries {
		if e == "assets/main.css" {
			hasMain = true
		}
		if e == "assets/secret.css" {
			hasSecret = true
		}
	}
	if !hasMain {
		t.Errorf("main.css should be present (line 2 negation wins over line 1)")
	}
	if hasSecret {
		t.Errorf("secret.css should be excluded (line 3 wins over line 2)")
	}
}

func TestThemeignore_SelfAlwaysExcluded(t *testing.T) {
	root := setupThemeDir(t)
	writeThemeignore(t, root, "*.bak\n")
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	for _, e := range zipEntries(t, out) {
		if e == ".themeignore" {
			t.Fatalf(".themeignore must always be excluded from zip")
		}
	}
}

func TestThemeignore_NoFile_FallsBackToV1Behavior(t *testing.T) {
	root := setupThemeDir(t)
	// no .themeignore at root
	out := filepath.Join(root, "test.zip")
	_, _ = Pack(root, out, PackOptions{})
	// Just ensure pack succeeded with the seven standard dirs
	entries := zipEntries(t, out)
	if len(entries) == 0 {
		t.Fatal("zip should be non-empty under v1 fallback")
	}
}

func TestThemeignore_ForceNoIgnore(t *testing.T) {
	root := setupThemeDir(t)
	writeThemeignore(t, root, "assets/*\n")
	out := filepath.Join(root, "test.zip")
	// IgnoreFile = "/dev/null" sentinel = force-disable
	_, _ = Pack(root, out, PackOptions{IgnoreFile: "/dev/null"})
	hasAssetsMain := false
	for _, e := range zipEntries(t, out) {
		if e == "assets/main.css" {
			hasAssetsMain = true
		}
	}
	if !hasAssetsMain {
		t.Fatalf("with --no-ignore equivalent, assets/main.css should ship")
	}
}

// TestThemeignore_GitCheckIgnoreParity checks parity against `git check-ignore`
// for negation, **, and anchoring patterns. Skipped if `git` is not in PATH.
func TestThemeignore_GitCheckIgnoreParity(t *testing.T) {
	if _, err := exec.LookPath("git"); err != nil {
		t.Skip("git not in PATH")
	}
	root := setupThemeDir(t)
	patterns := `# parity test
*.bak
**/draft-*.css
!assets/main.css
/draft.txt
node_modules/
`
	writeThemeignore(t, root, patterns)
	fixtures := []string{
		"assets/main.css",
		"assets/main.css.bak",
		"assets/draft-x.css",
		"assets/sub/draft-y.css",
		"draft.txt",
		"assets/draft.txt",
		"node_modules/foo.js",
	}
	for _, f := range fixtures {
		full := filepath.Join(root, f)
		_ = os.MkdirAll(filepath.Dir(full), 0o755)
		_ = os.WriteFile(full, []byte("x"), 0o644)
	}
	// `git check-ignore` reads .gitignore from the working tree, and requires
	// a git repo to anchor to. Set up an empty repo with the .themeignore
	// patterns copied to .gitignore so check-ignore consults them.
	initCmd := exec.Command("git", "init", "-q")
	initCmd.Dir = root
	if err := initCmd.Run(); err != nil {
		t.Skipf("git init failed in temp dir: %v", err)
	}
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte(patterns), 0o644); err != nil {
		t.Fatal(err)
	}
	// Run git check-ignore for each fixture. With -v, the output format is
	// "<source>:<line>:<pattern>\t<pathname>". A leading "!" in <pattern> means
	// the rule un-ignores the file. Exit code 0 = a rule matched (which can be
	// either ignore or un-ignore); exit code 1 = no rule matched.
	for _, f := range fixtures {
		cmd := exec.Command("git", "check-ignore", "--no-index", "-v", f)
		cmd.Dir = root
		out, err := cmd.CombinedOutput()
		gitIgnored := false
		if err == nil && len(out) > 0 {
			// Last colon-separated field before the TAB is the pattern.
			line := strings.TrimRight(string(out), "\n")
			tab := strings.Index(line, "\t")
			meta := line
			if tab >= 0 {
				meta = line[:tab]
			}
			// meta is "<source>:<line>:<pattern>"; pull pattern after the second ':'.
			idx1 := strings.Index(meta, ":")
			pattern := ""
			if idx1 >= 0 {
				rest := meta[idx1+1:]
				idx2 := strings.Index(rest, ":")
				if idx2 >= 0 {
					pattern = rest[idx2+1:]
				}
			}
			gitIgnored = !strings.HasPrefix(pattern, "!")
		}

		// our library decision
		gi, _ := loadIgnorer(root, "")
		ourIgnored := gi != nil && gi.MatchesPath(f)

		if gitIgnored != ourIgnored {
			t.Errorf("PARITY MISMATCH %s: git=%v ours=%v (git output: %s)", f, gitIgnored, ourIgnored, string(out))
		}
	}
}
