package themecmd

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
)

// helpFor renders a command's --help. A zero-valued Factory is safe: --help
// short-circuits before RunE.
func helpFor(t *testing.T, newCmd func(*cmdutil.Factory) *cobra.Command) string {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza"}
	c := newCmd(&cmdutil.Factory{})
	root.AddCommand(c)
	buf := &bytes.Buffer{}
	root.SetOut(buf)
	root.SetErr(buf)
	root.SetArgs([]string{c.Name(), "--help"})
	if err := root.Execute(); err != nil {
		t.Fatalf("help execute: %v", err)
	}
	return buf.String()
}

func TestHelp_Push(t *testing.T) {
	out := helpFor(t, newCmdPush)
	for _, want := range []string{"--theme-id", "-t", "-e", "--environment", "overwrites"} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("push help missing %q:\n%s", want, out)
		}
	}
}

func TestHelp_Pull(t *testing.T) {
	out := helpFor(t, newCmdPull)
	for _, want := range []string{"--theme-id", "-t", "themes list", "-e"} {
		if !strings.Contains(out, want) {
			t.Errorf("pull help missing %q:\n%s", want, out)
		}
	}
}

// TestHelp_Serve pins the LiveReload port, the dual-mode explanation, the
// dev-theme persistence path, overwrite semantics, the theme-dir requirement,
// the one-way-sync pointer, and that --theme-id reads as optional.
func TestHelp_Serve(t *testing.T) {
	out := helpFor(t, newCmdServe)
	for _, want := range []string{
		"--port",
		"development theme",
		".shoplazza/theme-state.json",
		"overwrites",
		"config/settings_schema.json",
		"themes pull",
		"-e",
	} {
		if !strings.Contains(strings.ToLower(out), strings.ToLower(want)) {
			t.Errorf("serve help missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Theme ID (required)") {
		t.Errorf("serve --theme-id must not be documented as required:\n%s", out)
	}
	if !strings.Contains(strings.ToLower(out), "omit") {
		t.Errorf("serve --theme-id should explain omitting it:\n%s", out)
	}
	if strings.Contains(out, "--no-livereload") {
		t.Errorf("serve must not expose --no-livereload:\n%s", out)
	}
}
