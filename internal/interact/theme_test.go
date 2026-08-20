package interact

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
)

// builder returns a fresh, un-initialised n-option form on every call.
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

// frameRows reports the rows f occupies at the given terminal height, counting
// the newline the renderer appends.
func frameRows(f *huh.Form, termRows int) int {
	f.Init()
	m, _ := f.Update(tea.WindowSizeMsg{Width: 80, Height: termRows})
	return len(strings.Split(strings.TrimRight(m.(*huh.Form).View(), "\n"), "\n")) + 1
}

// TestSizedFor_NeverFillsTheTerminal asserts the frame never fills the terminal.
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

// TestSizedFor_LeavesHuhAloneWhenItFits pins huh's own layout while it fits.
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

// TestSizedFor_NoTerminalDefersToHuh asserts h <= 0 leaves the height to huh.
func TestSizedFor_NoTerminalDefersToHuh(t *testing.T) {
	build := builder(22, "")
	for _, h := range []int{0, -1} {
		if got, want := frameRows(sizedFor(build, 80, h), 9999), frameRows(build(), 9999); got != want {
			t.Errorf("h=%d: sized %d rows, huh alone %d", h, got, want)
		}
	}
}

// TestNeededRows_DoesNotMutateBindings asserts the probe leaves the bound
// values alone.
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

// TestHuhCallsValidateOnceOnFocus pins the single focus-time Validate call that
// NotOnArrival skips.
func TestHuhCallsValidateOnceOnFocus(t *testing.T) {
	var calls []int // one entry per Validate call, holding the value's length
	var sel []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which domains?").
			Options(huh.NewOption("products", "products"), huh.NewOption("orders", "orders")).
			Validate(func(v []string) error {
				calls = append(calls, len(v))
				return nil
			}).
			Value(&sel),
	))

	pump := func(cmd tea.Cmd) {
		for i := 0; cmd != nil && i < 8; i++ {
			msg := cmd()
			if msg == nil {
				return
			}
			m, next := form.Update(msg)
			form = m.(*huh.Form)
			cmd = next
		}
	}

	pump(form.Init())
	if len(calls) != 1 {
		t.Fatalf("focus made %d Validate calls (values seen: %v), want exactly 1 — NotOnArrival assumes it can skip one", len(calls), calls)
	}

	// A cursor move must not count as an answer.
	before := len(calls)
	m, cmd := form.Update(tea.KeyMsg{Type: tea.KeyDown})
	form = m.(*huh.Form)
	pump(cmd)
	if len(calls) != before {
		t.Errorf("a cursor move made %d extra Validate calls, want 0", len(calls)-before)
	}

	// Enter must reach the validator.
	before = len(calls)
	m, cmd = form.Update(tea.KeyMsg{Type: tea.KeyEnter})
	form = m.(*huh.Form)
	pump(cmd)
	if len(calls) <= before {
		t.Error("enter made no Validate call; an on-submit error could never appear")
	}
}

// TestHuhCallsValidateOnGoingBack asserts going back also runs Validate, so a
// field validator cannot be made submit-only.
func TestHuhCallsValidateOnGoingBack(t *testing.T) {
	var calls int
	var store string
	var sel []string
	form := huh.NewForm(
		huh.NewGroup(huh.NewInput().Title("Which store?").Value(&store)),
		huh.NewGroup(huh.NewMultiSelect[string]().
			Title("Which domains?").
			Options(huh.NewOption("products", "products")).
			Validate(func(v []string) error { calls++; return nil }).
			Value(&sel)),
	).WithKeyMap(keyMap())

	pump := func(cmd tea.Cmd) {
		for i := 0; cmd != nil && i < 10; i++ {
			msg := cmd()
			if msg == nil {
				return
			}
			m, next := form.Update(msg)
			form = m.(*huh.Form)
			cmd = next
		}
	}
	send := func(msg tea.Msg) {
		m, cmd := form.Update(msg)
		form = m.(*huh.Form)
		pump(cmd)
	}

	pump(form.Init())
	send(tea.KeyMsg{Type: tea.KeyEnter}) // leave the store, focus the domains
	before := calls
	send(tea.KeyMsg{Type: tea.KeyEsc}) // ask to go back
	if calls == before {
		t.Error("going back made no Validate call — if that ever becomes true, a field validator CAN be made submit-only and NotOnArrival's residual disappears")
	}
}

// TestNotOnArrival covers the wrapper itself.
func TestNotOnArrival(t *testing.T) {
	boom := errors.New("boom")
	v := NotOnArrival(func(s string) error { return boom })

	if err := v("arrival"); err != nil {
		t.Errorf("first call returned %v, want nil: the screen must not arrive red", err)
	}
	for i := 0; i < 3; i++ {
		if err := v("later"); err != boom {
			t.Errorf("call %d returned %v, want the wrapped error", i+2, err)
		}
	}

	// Each wrapper is independent: two fields must not share one arrival.
	other := NotOnArrival(func(s string) error { return boom })
	if err := other("arrival"); err != nil {
		t.Errorf("a second wrapper returned %v on its own first call, want nil", err)
	}
}
