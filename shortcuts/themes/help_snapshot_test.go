package themes

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/cmdutil"
	"shoplazza-cli-v2/shortcuts/common"
)

// helpFor mounts the themes workflow shortcuts onto a fresh root cobra
// command, triggers `--help` for the supplied command path, and returns the
// captured help output. A zero-valued Factory is safe because `--help`
// short-circuits before cobra reaches RunE.
func helpFor(t *testing.T, cmdPath ...string) string {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza"}
	f := &cmdutil.Factory{}
	svc := &cobra.Command{Use: "themes"}
	root.AddCommand(svc)
	for _, s := range Shortcuts() {
		common.Mount(s, svc, f)
	}
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	args := append(append([]string{}, cmdPath...), "--help")
	root.SetArgs(args)
	if err := root.Execute(); err != nil {
		t.Fatalf("root.Execute(%v) returned error: %v", args, err)
	}
	return buf.String()
}

func TestHelp_Init(t *testing.T) {
	out := helpFor(t, "themes", "init")
	for _, want := range []string{"init", "--name", "Nova-2023"} {
		if !strings.Contains(out, want) {
			t.Errorf("init help missing %q in:\n%s", want, out)
		}
	}
}

func TestHelp_Package(t *testing.T) {
	out := helpFor(t, "themes", "package")
	if !strings.Contains(out, "--no-ignore") {
		t.Errorf("package help missing --no-ignore:\n%s", out)
	}
}

func TestHelp_Pull(t *testing.T) {
	out := helpFor(t, "themes", "pull")
	for _, want := range []string{"pull", "--theme-id", "-t", "themes list"} {
		if !strings.Contains(out, want) {
			t.Errorf("pull help missing %q:\n%s", want, out)
		}
	}
}

func TestHelp_Push(t *testing.T) {
	out := helpFor(t, "themes", "push")
	if !strings.Contains(out, "--theme-id") {
		t.Errorf("push help missing --theme-id:\n%s", out)
	}
}

func TestHelp_Preview(t *testing.T) {
	out := helpFor(t, "themes", "+preview")
	for _, want := range []string{"+preview", "--theme-id", "-t", "--oseid"} {
		if !strings.Contains(out, want) {
			t.Errorf("+preview help missing %q:\n%s", want, out)
		}
	}
}

// TestHelp_Share_HasNoThemeID: share is a non-destructive snapshot — it always
// uploads a fresh temporary theme and never takes a --theme-id. Overwriting an
// existing theme is `themes push`'s job; share must not expose a -t footgun.
func TestHelp_Share_HasNoThemeID(t *testing.T) {
	out := helpFor(t, "themes", "share")
	if strings.Contains(out, "--theme-id") {
		t.Errorf("share must NOT expose --theme-id (overwrite is push's job):\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "temporary") {
		t.Errorf("share help should describe the upload as a temporary preview:\n%s", out)
	}
}

func TestHelp_Serve_HasLivereloadPort(t *testing.T) {
	out := helpFor(t, "themes", "serve")
	if !strings.Contains(out, "--port") {
		t.Errorf("serve help missing --port:\n%s", out)
	}
	if strings.Contains(out, "--no-livereload") {
		t.Errorf("serve help must NOT expose --no-livereload:\n%s", out)
	}
}

// TestHelp_Serve_ExplainsDualMode: serve's long help must explain the two
// modes (default development theme vs explicit --theme-id), where the dev
// theme id is persisted, the overwrite semantics of the explicit mode, the
// theme-directory requirement, and the one-way (local → remote) sync.
func TestHelp_Serve_ExplainsDualMode(t *testing.T) {
	out := helpFor(t, "themes", "serve")
	for _, want := range []string{
		"development theme",           // default mode named
		".shoplazza/theme-state.json", // where the id is written back
		"overwrites",                  // explicit mode is destructive
		"config/settings_schema.json", // theme-directory requirement
		"themes pull",                 // editor changes are not synced back
		"serve [--theme-id <id>]",     // usage shows the flag as optional
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("serve help missing %q in:\n%s", want, out)
		}
	}
}

// TestHelp_Serve_ThemeIDFlagIsOptional: the --theme-id flag description must
// flag itself as optional and point at the development-theme default.
func TestHelp_Serve_ThemeIDFlagIsOptional(t *testing.T) {
	out := helpFor(t, "themes", "serve")
	if strings.Contains(out, "Theme ID (required)") {
		t.Errorf("serve --theme-id must no longer be documented as required:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "omit") {
		t.Errorf("serve --theme-id description should explain what omitting it does:\n%s", out)
	}
}
