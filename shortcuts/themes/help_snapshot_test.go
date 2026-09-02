package themes

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// helpFor mounts the themes shortcuts onto a fresh root cobra command (each
// under its own Service path), triggers `--help` for the supplied command
// path, and returns the captured help output. A zero-valued Factory is safe because `--help`
// short-circuits before cobra reaches RunE.
func helpFor(t *testing.T, cmdPath ...string) string {
	t.Helper()
	root := &cobra.Command{Use: "shoplazza"}
	f := &cmdutil.Factory{}
	for _, s := range Shortcuts() {
		// Mount under the shortcut's own Service path (e.g. "themes block").
		parent := root
		for _, seg := range strings.Fields(s.Service) {
			var child *cobra.Command
			for _, c := range parent.Commands() {
				if c.Name() == seg {
					child = c
					break
				}
			}
			if child == nil {
				child = &cobra.Command{Use: seg}
				parent.AddCommand(child)
			}
			parent = child
		}
		common.Mount(s, parent, f)
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

func TestHelp_BlockEdit(t *testing.T) {
	out := helpFor(t, "themes", "block", "+edit")
	for _, want := range []string{"+edit", "--session", "--content", "--id", "--template", "--target", "--settings", "--ops", "branched", "revert-gen"} {
		if !strings.Contains(out, want) {
			t.Errorf("block +edit help missing %q in:\n%s", want, out)
		}
	}
	if flags := flagsSection(out); strings.Contains(flags, "--promote") {
		t.Errorf("block +edit must not expose --promote (saving is themes +edit's job):\n%s", flags)
	}
}

// flagsSection returns the "Flags:" part of a help output.
func flagsSection(out string) string {
	if i := strings.Index(out, "Flags:"); i >= 0 {
		return out[i:]
	}
	return ""
}

func TestHelp_BlockGet(t *testing.T) {
	out := helpFor(t, "themes", "block", "+get")
	for _, want := range []string{"+get", "--session", "--id", "--section", "--template", "--with-content", "ref_count"} {
		if !strings.Contains(out, want) {
			t.Errorf("block +get help missing %q in:\n%s", want, out)
		}
	}
}

// TestHelp_EditIsTopLevelOnly: the page-editing +edit stays under themes, the
// block-editing +edit under themes block — the two must not shadow each other.
func TestHelp_EditIsTopLevelOnly(t *testing.T) {
	top := helpFor(t, "themes", "+edit")
	if !strings.Contains(top, "--ops") || !strings.Contains(top, "--promote") {
		t.Errorf("themes +edit help lost its flags:\n%s", top)
	}
	block := flagsSection(helpFor(t, "themes", "block", "+edit"))
	if strings.Contains(block, "--promote") || !strings.Contains(block, "--content") {
		t.Errorf("themes block +edit help resolved to the wrong command:\n%s", block)
	}
}
