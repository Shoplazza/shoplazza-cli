package update

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/metasync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/skillsync"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
)

// TestMain stubs both refreshes so runUpdate tests never hit the network, clears
// the opt-out envs a developer or CI job may export, and isolates the home so
// the skills read as "not installed" whatever is under the real one.
func TestMain(m *testing.M) {
	os.Unsetenv(metasync.EnvDisable)
	os.Unsetenv(skillsync.EnvSkip)
	metaRefresh = func(context.Context, string) (metasync.Result, error) {
		return metasync.Result{OldRevision: "r0"}, nil
	}
	skillRefresh = func(context.Context) (skillsync.Result, error) {
		return skillsync.Result{Installed: true, Count: 1, Ran: true}, nil
	}
	testenv.RunMainIsolated(m)
}

// seedSkills makes the skills directory look like the user installed n skills.
func seedSkills(t *testing.T, names ...string) {
	t.Helper()
	dir := testenv.IsolateSkillsDir(t)
	for _, n := range names {
		if err := os.MkdirAll(filepath.Join(dir, n), 0o755); err != nil {
			t.Fatalf("mkdir %s: %v", n, err)
		}
		if err := os.WriteFile(filepath.Join(dir, n, "SKILL.md"), []byte("x"), 0o644); err != nil {
			t.Fatalf("write %s: %v", n, err)
		}
	}
}

func TestNewCmdUpdate_Structure(t *testing.T) {
	cmd := NewCmdUpdate(nil)
	if cmd.Use != "update" {
		t.Errorf("Use = %q, want update", cmd.Use)
	}
	if cmd.Flags().Lookup("check") == nil {
		t.Error("expected --check flag")
	}
}

func TestUpToDate(t *testing.T) {
	cases := []struct {
		latest, current string
		err             error
		want            bool
	}{
		{"2.0.1", "2.0.1", nil, true},         // equal → up to date
		{"2.0.2", "2.0.1", nil, false},        // newer available
		{"2.0.1", "2.0.2", nil, true},         // local ahead → up to date
		{"v2.0.1", "2.0.1", nil, true},        // v-prefix handled by IsNewer
		{"", "2.0.1", errors.New("x"), false}, // lookup failed → allow update
		{"", "2.0.1", nil, false},             // empty latest → allow update
	}
	for _, c := range cases {
		if got := upToDate(c.latest, c.current, c.err); got != c.want {
			t.Errorf("upToDate(%q,%q,%v)=%v want %v", c.latest, c.current, c.err, got, c.want)
		}
	}
}

// fakeOps records install invocations and serves canned latest-version results.
type fakeOps struct {
	latestVer     string
	latestErr     error
	installCalled bool
	installErr    error
}

func (f *fakeOps) build() npmOps {
	return npmOps{
		lookPath: func() (string, error) { return "npm", nil },
		latest:   func(context.Context, string) (string, error) { return f.latestVer, f.latestErr },
		install: func(_ context.Context, _ string, out io.Writer) error {
			f.installCalled = true
			io.WriteString(out, "changed 1 package\n")
			return f.installErr
		},
	}
}

func decodeBody(t *testing.T, b []byte) map[string]any {
	t.Helper()
	var m map[string]any
	if err := json.Unmarshal(b, &m); err != nil {
		t.Fatalf("stdout is not JSON: %v (%s)", err, b)
	}
	return m
}

func TestRunUpdate_AlreadyLatest_SkipsInstall(t *testing.T) {
	f := &fakeOps{latestVer: "2.0.1"}
	var out, errW bytes.Buffer
	if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if f.installCalled {
		t.Error("install must not run when already up to date")
	}
	if !strings.Contains(errW.String(), "up to date") {
		t.Errorf("stderr should report up to date; got %q", errW.String())
	}
	if decodeBody(t, out.Bytes())["updated"] != false {
		t.Errorf("body.updated should be false; got %s", out.String())
	}
}

func TestRunUpdate_UpdateAvailable_RunsInstallAndReports(t *testing.T) {
	f := &fakeOps{latestVer: "2.0.2"}
	var out, errW bytes.Buffer
	if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if !f.installCalled {
		t.Error("install must run when an update is available")
	}
	es := errW.String()
	if !strings.Contains(es, "Updating "+npmPackage) {
		t.Errorf("stderr should show the spinner label; got %q", es)
	}
	if !strings.Contains(es, "Updated") || !strings.Contains(es, "2.0.2") {
		t.Errorf("stderr should report the new version; got %q", es)
	}
	body := decodeBody(t, out.Bytes())
	if body["updated"] != true || body["latest"] != "2.0.2" || body["previous"] != "2.0.1" {
		t.Errorf("body mismatch: %s", out.String())
	}
	if body["meta_updated"] != false || body["meta_revision"] != "r0" {
		t.Errorf("body should carry metadata refresh outcome: %s", out.String())
	}
}

func TestRunUpdate_MetaRefreshOutcomes(t *testing.T) {
	saved := metaRefresh
	t.Cleanup(func() { metaRefresh = saved })

	t.Run("updated metadata reported with post-install version", func(t *testing.T) {
		var gotVersion string
		metaRefresh = func(_ context.Context, version string) (metasync.Result, error) {
			gotVersion = version
			return metasync.Result{OldRevision: "r0", NewRevision: "r1", Updated: true}, nil
		}
		f := &fakeOps{latestVer: "2.0.2"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if gotVersion != "2.0.2" {
			t.Errorf("refresh must use the just-installed version, got %q", gotVersion)
		}
		body := decodeBody(t, out.Bytes())
		if body["meta_updated"] != true || body["meta_revision"] != "r1" {
			t.Errorf("body mismatch: %s", out.String())
		}
	})

	t.Run("refresh failure never fails the command", func(t *testing.T) {
		metaRefresh = func(context.Context, string) (metasync.Result, error) {
			return metasync.Result{}, errors.New("boom")
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("meta refresh failure must not fail update: %v", err)
		}
		body := decodeBody(t, out.Bytes())
		if body["meta_error"] != "boom" || body["ok"] != true {
			t.Errorf("body mismatch: %s", out.String())
		}
	})

	t.Run("disable env skips refresh", func(t *testing.T) {
		called := false
		metaRefresh = func(context.Context, string) (metasync.Result, error) {
			called = true
			return metasync.Result{}, nil
		}
		t.Setenv(metasync.EnvDisable, "1")
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("disable env must suppress the metadata refresh")
		}
		if _, ok := decodeBody(t, out.Bytes())["meta_updated"]; ok {
			t.Error("suppressed refresh must not add meta keys")
		}
	})

	t.Run("npm missing still refreshes metadata", func(t *testing.T) {
		var gotVersion string
		metaRefresh = func(_ context.Context, version string) (metasync.Result, error) {
			gotVersion = version
			return metasync.Result{}, nil
		}
		ops := npmOps{lookPath: func() (string, error) { return "", errors.New("not found") }}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, ops); err == nil {
			t.Fatal("npm missing must still be an error")
		}
		if gotVersion != "2.0.1" {
			t.Errorf("metadata half must run despite npm missing, got version %q", gotVersion)
		}
	})

	t.Run("install failure still refreshes metadata", func(t *testing.T) {
		called := false
		metaRefresh = func(context.Context, string) (metasync.Result, error) {
			called = true
			return metasync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.2", installErr: errors.New("EACCES")}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err == nil {
			t.Fatal("install failure must still be an error")
		}
		if !called {
			t.Error("metadata half must run despite install failure")
		}
	})

	t.Run("check-only skips refresh", func(t *testing.T) {
		called := false
		metaRefresh = func(context.Context, string) (metasync.Result, error) {
			called = true
			return metasync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", true, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("--check must not force a metadata refresh")
		}
	})
}

func TestRunUpdate_SkillOutcomes(t *testing.T) {
	saved := skillRefresh
	t.Cleanup(func() { skillRefresh = saved })

	// The gap this whole feature closes: an already-current binary never runs
	// npm, so the postinstall hook never fires and the skills used to go stale.
	t.Run("already latest refreshes the installed skills", func(t *testing.T) {
		seedSkills(t, "shoplazza-common", "shoplazza-orders")
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{Installed: true, Count: 2, Ran: true}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if !called {
			t.Error("an up-to-date binary must still refresh the skills")
		}
		if decodeBody(t, out.Bytes())["skill_installed"] != true {
			t.Errorf("body should report the skills as installed: %s", out.String())
		}
		if !strings.Contains(errW.String(), "Refreshed 2 skill(s)") {
			t.Errorf("stderr should report the refresh; got %q", errW.String())
		}
	})

	t.Run("not installed is reported without shelling out", func(t *testing.T) {
		testenv.IsolateSkillsDir(t)
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("skills the user never installed must not be pulled")
		}
		if decodeBody(t, out.Bytes())["skill_installed"] != false {
			t.Errorf("body should report skill_installed=false: %s", out.String())
		}
		if strings.Contains(errW.String(), "skill") {
			t.Errorf("stderr must stay quiet when the skills are not installed; got %q", errW.String())
		}
	})

	// Read as "none installed", a broken dir would stop every refresh, silently.
	t.Run("unreadable skills dir warns instead of passing as not installed", func(t *testing.T) {
		if runtime.GOOS == "windows" {
			t.Skip("chmod 0000 does not deny access on Windows")
		}
		if os.Geteuid() == 0 {
			t.Skip("root reads any directory")
		}
		dir := testenv.IsolateSkillsDir(t)
		if err := os.Chmod(dir, 0o000); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { os.Chmod(dir, 0o755) })
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("a dir we cannot read must not be handed to npx")
		}
		if !strings.Contains(errW.String(), "cannot read the Agent Skills directory") {
			t.Errorf("stderr should surface the unreadable dir; got %q", errW.String())
		}
		if decodeBody(t, out.Bytes())["skill_installed"] != false {
			t.Errorf("body should not claim skills are installed: %s", out.String())
		}
	})

	t.Run("refresh failure never fails the command", func(t *testing.T) {
		seedSkills(t, "shoplazza-common")
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			return skillsync.Result{Installed: true, Count: 1, Ran: true, Output: "npm ERR! boom"}, errors.New("boom")
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("skill refresh failure must not fail update: %v", err)
		}
		body := decodeBody(t, out.Bytes())
		if body["skill_installed"] != true || body["ok"] != true {
			t.Errorf("body mismatch: %s", out.String())
		}
		// npx output stays its own block, so the hint below isn't buried in it.
		if !strings.Contains(errW.String(), "npm ERR! boom\nwarning: skill refresh did not complete") {
			t.Errorf("npx output should precede the warning as its own line; got %q", errW.String())
		}
		if !strings.Contains(errW.String(), "manually") {
			t.Errorf("stderr should carry the manual command; got %q", errW.String())
		}
	})

	t.Run("skip env suppresses the whole step", func(t *testing.T) {
		seedSkills(t, "shoplazza-common")
		t.Setenv(skillsync.EnvSkip, "1")
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("skip env must suppress the skill refresh")
		}
		if _, ok := decodeBody(t, out.Bytes())["skill_installed"]; ok {
			t.Errorf("a suppressed step must add no skill key: %s", out.String())
		}
	})

	// npm's postinstall hook already refreshed them; pulling again would be a
	// second download of the same content.
	t.Run("install path reports without refreshing again", func(t *testing.T) {
		seedSkills(t, "shoplazza-common")
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.2"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("the install path must not refresh the skills a second time")
		}
		if decodeBody(t, out.Bytes())["skill_installed"] != true {
			t.Errorf("body should still report the skills: %s", out.String())
		}
	})

	t.Run("check-only reports without refreshing", func(t *testing.T) {
		seedSkills(t, "shoplazza-common")
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		f := &fakeOps{latestVer: "2.0.1"}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", true, f.build()); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if called {
			t.Error("--check must not refresh the skills")
		}
		if decodeBody(t, out.Bytes())["skill_installed"] != true {
			t.Errorf("--check should still report the state: %s", out.String())
		}
	})

	t.Run("npm missing skips the skill step", func(t *testing.T) {
		seedSkills(t, "shoplazza-common")
		called := false
		skillRefresh = func(context.Context) (skillsync.Result, error) {
			called = true
			return skillsync.Result{}, nil
		}
		ops := npmOps{lookPath: func() (string, error) { return "", errors.New("not found") }}
		var out, errW bytes.Buffer
		if err := runUpdate(context.Background(), &out, &errW, "json", "2.0.1", false, ops); err == nil {
			t.Fatal("npm missing must still be an error")
		}
		if called {
			t.Error("no npm means no npx — the skill step must not run")
		}
	})
}
