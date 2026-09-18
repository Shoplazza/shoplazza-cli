package themes

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
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

// push / pull / serve help moved to cmd/theme (they migrated to plain-cobra
// commands); their help assertions live in cmd/theme/help_test.go now.

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

