package interact

import (
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// builder returns a fresh form with n options, so every call gets an
// un-initialised one (sizedFor probes with one and returns another).
func builder(n int, desc string) func() *huh.Form {
	return func() *huh.Form {
		var v []string
		opts := make([]huh.Option[string], 0, n)
		for i := 0; i < n; i++ {
			opts = append(opts, huh.NewOption(fmt.Sprintf("option-%02d", i+1), fmt.Sprint(i)))
		}
		s := huh.NewMultiSelect[string]().
			Title("Which domains do you need access to?").
			Options(opts...).
			Value(&v)
		if desc != "" {
			s = s.Description(desc)
		}
		return huh.NewForm(huh.NewGroup(s))
	}
}

// frameRows renders a form against a terminal of the given height and reports
// the total rows it occupies: the frame plus the newline the renderer appends.
// That total exceeding the terminal height is precisely the bug — the terminal
// scrolls and the title goes with it.
func frameRows(f *huh.Form, termRows int) int {
	f.Init()
	m, _ := f.Update(tea.WindowSizeMsg{Width: 80, Height: termRows})
	return len(strings.Split(strings.TrimRight(m.(*huh.Form).View(), "\n"), "\n")) + 1
}

// TestSizedFor_NeverFillsTheTerminal is the regression test for the bug that
// cost the most time in this feature: with no height set, huh takes
// min(content, terminal), which is right until the content does not fit — then
// it saturates at the full terminal height, the renderer's trailing newline
// scrolls everything up one row, and the title scrolls off screen.
func TestSizedFor_NeverFillsTheTerminal(t *testing.T) {
	cases := []struct {
		name  string
		build func() *huh.Form
	}{
		{"2 options", builder(2, "")},
		{"11 options with a description", builder(11, "Each domain grants the scopes its commands need.")},
		{"22 options", builder(22, "")},
	}
	for _, c := range cases {
		for _, termRows := range []int{8, 12, 20, 40, 60} {
			t.Run(fmt.Sprintf("%s/term%d", c.name, termRows), func(t *testing.T) {
				got := frameRows(sizedFor(c.build, 80, termRows), termRows)
				if got > termRows {
					t.Errorf("frame occupies %d rows in a %d-row terminal: it scrolls, and the title goes first", got, termRows)
				}
			})
		}
	}
}

// TestSizedFor_LeavesHuhAloneWhenItFits pins the other half of the rule. huh's
// own layout is correct and tight while the content fits, so we must not set a
// height there: doing so unconditionally (WithHeight(termRows-2)) would stretch
// a two-option form to 58 rows on a 60-row terminal.
func TestSizedFor_LeavesHuhAloneWhenItFits(t *testing.T) {
	build := builder(2, "")
	var rows []int
	for _, termRows := range []int{20, 40, 60} {
		rows = append(rows, frameRows(sizedFor(build, 80, termRows), termRows))
	}
	for _, got := range rows[1:] {
		if got != rows[0] {
			t.Fatalf("a form that fits must render identically at any terminal height, got %v", rows)
		}
	}
	if unsized := frameRows(build(), 40); rows[0] != unsized {
		t.Errorf("a form that fits must match huh's own layout: sized %d, unsized %d", rows[0], unsized)
	}
}

// TestSizedFor_NoTerminalDefersToHuh covers the piped case: with no terminal
// there is no height to cap against, so the decision belongs to huh.
func TestSizedFor_NoTerminalDefersToHuh(t *testing.T) {
	build := builder(22, "")
	for _, h := range []int{0, -1} {
		if got, want := frameRows(sizedFor(build, 80, h), 9999), frameRows(build(), 9999); got != want {
			t.Errorf("h=%d: sized %d rows, huh alone %d", h, got, want)
		}
	}
}

// TestNeededRows_DoesNotMutateBindings guards the probe. sizedFor renders the form
// once to ask how tall it wants to be; if that wrote answers back into the
// caller's variables, every pre-filled flag would be silently clobbered.
func TestNeededRows_DoesNotMutateBindings(t *testing.T) {
	text := "my-store.myshoplaza.com"
	sel := []string{"products"}
	build := func() *huh.Form {
		return huh.NewForm(
			huh.NewGroup(huh.NewInput().Title("Which store?").Value(&text)),
			huh.NewGroup(huh.NewMultiSelect[string]().Title("Which domains?").
				Options(huh.NewOption("products", "products"), huh.NewOption("orders", "orders")).
				Value(&sel)),
		)
	}
	_ = neededRows(build, 80)
	if text != "my-store.myshoplaza.com" {
		t.Errorf("probe mutated the input binding: %q", text)
	}
	if len(sel) != 1 || sel[0] != "products" {
		t.Errorf("probe mutated the multi-select binding: %v", sel)
	}
}
