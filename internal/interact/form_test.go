package interact

import (
	"errors"
	"fmt"
	"io"
	"strings"
	"testing"
	"time"

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

// pump mirrors bubbletea's loop: every message passes the filter NewForm
// installs before it reaches the form.
func pump(form **huh.Form, msg tea.Msg) {
	queue := []tea.Msg{msg}
	for n := 0; len(queue) > 0 && n < 100; n++ {
		m := armSubmit(*form, queue[0])
		queue = queue[1:]
		mod, cmd := (*form).Update(m)
		*form = mod.(*huh.Form)
		for _, next := range collect(cmd) {
			queue = append(queue, next)
		}
	}
}

// collect runs cmd and returns the messages it produced, dropping any that does
// not arrive promptly (textinput's cursor blink only fires after ~530ms).
func collect(cmd tea.Cmd) []tea.Msg {
	if cmd == nil {
		return nil
	}
	ch := make(chan tea.Msg, 1)
	go func() { ch <- cmd() }()
	select {
	case msg := <-ch:
		if batch, ok := msg.(tea.BatchMsg); ok {
			var out []tea.Msg
			for _, c := range batch {
				out = append(out, collect(c)...)
			}
			return out
		}
		if msg == nil {
			return nil
		}
		return []tea.Msg{msg}
	case <-time.After(250 * time.Millisecond):
		return nil
	}
}

// TestArmSubmit pins which keys count as "submitting this field". Everything
// else, including messages huh sends itself, must disarm.
func TestArmSubmit(t *testing.T) {
	t.Cleanup(func() { submitting.Store(false) })
	for _, tc := range []struct {
		name string
		msg  tea.Msg
		want bool
	}{
		{"enter", tea.KeyMsg{Type: tea.KeyEnter}, true},
		{"tab", tea.KeyMsg{Type: tea.KeyTab}, true},
		{"esc", tea.KeyMsg{Type: tea.KeyEsc}, false},
		{"shift+tab", tea.KeyMsg{Type: tea.KeyShiftTab}, false},
		{"x toggles", tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}, false},
		{"space toggles", tea.KeyMsg{Type: tea.KeySpace}, false},
		{"cursor down", tea.KeyMsg{Type: tea.KeyDown}, false},
		{"ctrl+a selects all", tea.KeyMsg{Type: tea.KeyCtrlA}, false},
		{"a resize is not a key", tea.WindowSizeMsg{Width: 80, Height: 24}, false},
	} {
		t.Run(tc.name, func(t *testing.T) {
			submitting.Store(!tc.want) // start from the wrong value
			armSubmit(nil, tc.msg)
			if got := submitting.Load(); got != tc.want {
				t.Errorf("armSubmit(%s) left submitting = %v, want %v", tc.name, got, tc.want)
			}
		})
	}
}

// TestOnlyOnSubmit covers the wrapper itself.
func TestOnlyOnSubmit(t *testing.T) {
	t.Cleanup(func() { submitting.Store(false) })
	boom := errors.New("boom")
	v := OnlyOnSubmit(func(string) error { return boom })

	submitting.Store(false)
	for _, when := range []string{"on focus", "on a toggle", "on going back"} {
		if err := v("value"); err != nil {
			t.Errorf("%s: returned %v, want nil", when, err)
		}
	}
	submitting.Store(true)
	if err := v("value"); err != boom {
		t.Errorf("on submit: returned %v, want the wrapped error", err)
	}
}

// TestHuhValidatesOutsideSubmit pins the huh behavior OnlyOnSubmit exists for:
// focus and toggles validate too. If this ever stops being true the wrapper is
// no longer needed.
func TestHuhValidatesOutsideSubmit(t *testing.T) {
	var calls int
	var sel []string
	form := huh.NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which domains?").
			Options(huh.NewOption("products", "products"), huh.NewOption("orders", "orders")).
			Validate(func([]string) error { calls++; return nil }).
			Value(&sel),
	))
	if form.Init(); calls == 0 {
		t.Error("focus made no Validate call")
	}
	before := calls
	pump(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	if calls == before {
		t.Error("a toggle made no Validate call")
	}
}

// twoScreenForm is the shape auth login builds when both screens are planned: a
// store input, then a domain picker that requires one once a store is given.
func twoScreenForm(wrap bool) *huh.Form {
	store := "my-store.myshoplaza.com"
	var sel []string
	check := func(v []string) error {
		if len(v) == 0 && store != "" {
			return errors.New("needs at least one domain")
		}
		return nil
	}
	if wrap {
		check = OnlyOnSubmit(check)
	}
	return huh.NewForm(
		huh.NewGroup(huh.NewInput().Title("Which store?").Value(&store)),
		huh.NewGroup(huh.NewMultiSelect[string]().
			Title("Which domains?").
			Options(huh.NewOption("products", "products"), huh.NewOption("orders", "orders")).
			Validate(check).Value(&sel)),
	).WithKeyMap(keyMap())
}

// TestOnlyOnSubmit_ErrorOnEnterAndEscStillGoesBack is the regression this wrapper
// was written for: huh refuses to leave a field holding an error, so an error
// raised anywhere but on enter strands the user on the screen.
func TestOnlyOnSubmit_ErrorOnEnterAndEscStillGoesBack(t *testing.T) {
	const errMsg = "needs at least one domain"
	reach := func(wrap bool) *huh.Form {
		form := twoScreenForm(wrap)
		for _, msg := range collect(form.Init()) {
			pump(&form, msg)
		}
		pump(&form, tea.WindowSizeMsg{Width: 100, Height: 40})
		pump(&form, tea.KeyMsg{Type: tea.KeyEnter}) // leave the store, arrive at the domains
		return form
	}

	form := reach(true)
	if strings.Contains(form.View(), errMsg) {
		t.Error("the domains screen arrived showing an error")
	}
	pump(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}})
	pump(&form, tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{'x'}}) // select then deselect
	if strings.Contains(form.View(), errMsg) {
		t.Error("toggling raised the error before enter")
	}
	pump(&form, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(form.View(), errMsg) {
		t.Fatalf("enter with nothing selected showed no error:\n%s", form.View())
	}
	pump(&form, tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(form.View(), "Which store?") {
		t.Errorf("esc did not go back while the error was showing:\n%s", form.View())
	}

	// Unwrapped, the same esc is swallowed — that is the behavior being fixed.
	bare := reach(false)
	pump(&bare, tea.KeyMsg{Type: tea.KeyEnter})
	pump(&bare, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(bare.View(), "Which store?") {
		t.Error("an unwrapped validator no longer blocks esc; OnlyOnSubmit may be unnecessary")
	}
}

// TestNewForm_InstallsTheSubmitFilter guards the wiring end to end: without the
// filter the flag never arms and every validator silently passes, which fails
// open. WithProgramOptions assigns teaOptions, so NewForm's call order matters.
func TestNewForm_InstallsTheSubmitFilter(t *testing.T) {
	t.Setenv("TERM", "xterm-256color") // TERM=dumb would skip the tea program
	var errs int
	var sel []string
	submitting.Store(true) // stale flag from a previous form
	form := NewForm(huh.NewGroup(
		huh.NewMultiSelect[string]().
			Title("Which domains?").
			Options(huh.NewOption("products", "products"), huh.NewOption("orders", "orders")).
			Validate(OnlyOnSubmit(func(v []string) error {
				if len(v) == 0 {
					errs++
					return errors.New("pick one")
				}
				return nil
			})).
			Value(&sel),
	))
	if submitting.Load() {
		t.Fatal("NewForm left the submit flag armed; the next focus validation would fire")
	}
	form = form.WithInput(strings.NewReader("\rx\r")).WithOutput(io.Discard)

	// enter (refused), x (selects), enter (submits).
	done := make(chan error, 1)
	go func() { done <- form.Run() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("form.Run: %v", err)
		}
	case <-time.After(5 * time.Second):
		t.Fatal("form.Run did not finish; the input script no longer completes the form")
	}
	// Without the filter nothing ever arms, so the first enter submits an empty
	// selection: errs stays 0 and sel stays empty. (An armed enter validates
	// twice — huh validates on the value update and again before advancing.)
	if errs == 0 {
		t.Error("the validator never fired: the submit filter is not installed")
	}
	if len(sel) != 1 || sel[0] != "products" {
		t.Errorf("selection = %v, want [products]: the first enter was not refused", sel)
	}
}

// TestOnlyOnSubmit_EscLeavesAnInputThatWouldNotValidate covers the other way huh
// strands the user: Input.Prev does not validate, but Input.Blur does, and the
// prevGroup transition is then refused because the group holds an error. This is
// app init's name screen, reached from the app picker.
func TestOnlyOnSubmit_EscLeavesAnInputThatWouldNotValidate(t *testing.T) {
	const create = "\x00create"
	const errMsg = "app name is required"

	reach := func(wrap bool) *huh.Form {
		choice, name := create, ""
		check := func(s string) error {
			if strings.TrimSpace(s) == "" {
				return errors.New(errMsg)
			}
			return nil
		}
		if wrap {
			check = OnlyOnSubmit(check)
		}
		form := huh.NewForm(
			huh.NewGroup(huh.NewSelect[string]().Title("Which app?").
				Options(huh.NewOption("Create a new app…", create), huh.NewOption("MyApp", "cid_x")).
				Value(&choice)),
			huh.NewGroup(huh.NewInput().Title("What is the app called?").
				Placeholder("My App").Validate(check).Value(&name)).
				WithHideFunc(func() bool { return choice != create }),
		).WithKeyMap(keyMap())
		submitting.Store(false)
		for _, msg := range collect(form.Init()) {
			pump(&form, msg)
		}
		pump(&form, tea.WindowSizeMsg{Width: 100, Height: 40})
		pump(&form, tea.KeyMsg{Type: tea.KeyEnter}) // take the create branch
		return form
	}

	form := reach(true)
	if strings.Contains(form.View(), errMsg) {
		t.Errorf("the name screen arrived showing an error:\n%s", form.View())
	}
	pump(&form, tea.KeyMsg{Type: tea.KeyEsc})
	if !strings.Contains(form.View(), "Which app?") {
		t.Errorf("esc did not go back from the name screen:\n%s", form.View())
	}
	if strings.Contains(form.View(), errMsg) {
		t.Errorf("going back left the validator's error on screen:\n%s", form.View())
	}

	// Enter must still refuse a blank name.
	form = reach(true)
	pump(&form, tea.KeyMsg{Type: tea.KeyEnter})
	if !strings.Contains(form.View(), errMsg) {
		t.Errorf("enter on a blank name showed no error:\n%s", form.View())
	}

	// Unwrapped, esc is swallowed and the error appears instead.
	bare := reach(false)
	pump(&bare, tea.KeyMsg{Type: tea.KeyEsc})
	if strings.Contains(bare.View(), "Which app?") {
		t.Error("an unwrapped Input validator no longer blocks esc; the wrapper may be unnecessary")
	}
}
