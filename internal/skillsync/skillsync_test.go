package skillsync

import (
	"context"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"
)

// isolateHome points the home directory (both names os.UserHomeDir consults) at
// a fresh temp dir and returns the skills path under it.
func isolateHome(t *testing.T) string {
	t.Helper()
	home := t.TempDir()
	t.Setenv("HOME", home)
	t.Setenv("USERPROFILE", home)
	return filepath.Join(home, ".agents", "skills")
}

// seedSkills creates name/SKILL.md under a fresh, isolated skills dir.
func seedSkills(t *testing.T, names ...string) string {
	t.Helper()
	dir := isolateHome(t)
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
		if err := os.WriteFile(filepath.Join(dir, n, "SKILL.md"), []byte("---\nname: "+n+"\n---\n"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
	return dir
}

// The skills location follows the home directory and nothing else — that is
// what keeps it in step with where `npx skills add -g` writes.
func TestDir_FollowsHome(t *testing.T) {
	want := isolateHome(t)
	if got := Dir(); got != want {
		t.Errorf("Dir() = %q, want %q", got, want)
	}
}

func TestInstalledNames_FiltersAndSorts(t *testing.T) {
	dir := seedSkills(t, "shoplazza-orders", "shoplazza-common")
	// A foreign skill and a shoplazza-prefixed directory without SKILL.md must
	// both be ignored — detection is "our prefix AND a real skill file".
	if err := os.MkdirAll(filepath.Join(dir, "some-other-skill"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, "some-other-skill", "SKILL.md"), []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(dir, "shoplazza-empty"), 0o755); err != nil {
		t.Fatal(err)
	}

	got, err := installedNames()
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	want := []string{"shoplazza-common", "shoplazza-orders"}
	if strings.Join(got, ",") != strings.Join(want, ",") {
		t.Errorf("installedNames() = %v, want %v", got, want)
	}
}

func TestInstalled_EmptyAndMissingDir(t *testing.T) {
	// Missing: an isolated home has no .agents/skills at all.
	dir := isolateHome(t)
	if installed, err := Installed(); installed || err != nil {
		t.Errorf("a missing skills dir must read as (false, nil), got (%v, %v)", installed, err)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	if installed, err := Installed(); installed || err != nil {
		t.Errorf("an empty skills dir must read as (false, nil), got (%v, %v)", installed, err)
	}
}

// An unreadable dir must not masquerade as "never installed".
func TestInstalled_UnreadableDirIsAnError(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("chmod 0000 does not deny access on Windows")
	}
	if os.Geteuid() == 0 {
		t.Skip("root reads any directory")
	}
	dir := seedSkills(t, "shoplazza-common")
	if err := os.Chmod(dir, 0o000); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.Chmod(dir, 0o755) })

	installed, err := Installed()
	if err == nil {
		t.Fatal("an unreadable skills dir must surface an error")
	}
	if installed {
		t.Error("an unreadable dir cannot claim skills are installed")
	}
}

// Refresh returns before it would even look for npx. An empty PATH makes that
// observable: reaching the exec would fail LookPath and surface an error.
func TestRefresh_NotInstalled_NeverShellsOut(t *testing.T) {
	isolateHome(t)
	t.Setenv("PATH", "")
	res, err := Refresh(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Installed || !res.Ran || res.Count != 0 {
		t.Errorf("want a ran-but-nothing-installed result, got %+v", res)
	}
}

func TestRefresh_SkipEnvSuppressesEverything(t *testing.T) {
	seedSkills(t, "shoplazza-common")
	t.Setenv(EnvSkip, "1")
	t.Setenv("PATH", "")
	res, err := Refresh(context.Background())
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if res.Ran || res.Installed {
		t.Errorf("skip env must suppress the whole step, got %+v", res)
	}
}

// Installed skills plus no npx is a reported failure, and the result still says
// they are installed — so callers can tell "tried and failed" from "nothing to do".
func TestRefresh_InstalledButNoNpx_ReportsFailure(t *testing.T) {
	seedSkills(t, "shoplazza-common", "shoplazza-orders")
	t.Setenv("PATH", "")
	res, err := Refresh(context.Background())
	if err == nil {
		t.Fatal("expected an error when npx is unavailable")
	}
	if !res.Installed || res.Count != 2 {
		t.Errorf("failure must still report what was installed, got %+v", res)
	}
}

func TestSourceAndManualCommand(t *testing.T) {
	seedSkills(t, "shoplazza-common")
	if Source() != defaultSource {
		t.Errorf("Source() = %q, want %q", Source(), defaultSource)
	}
	t.Setenv(EnvSource, "./local/path")
	got := ManualCommand()
	if !strings.Contains(got, "./local/path") || !strings.Contains(got, "shoplazza-common") {
		t.Errorf("ManualCommand() = %q, want the source and the installed names", got)
	}
}
