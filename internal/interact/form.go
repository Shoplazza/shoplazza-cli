package interact

import (
	"errors"
	"os"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/x/term"
)

// NewForm wires the plumbing every interactive command needs: keystrokes from
// stdin, the UI on stderr (never stdout, which carries the result envelope),
// the brand theme, esc-goes-back. Pass every step as a group of ONE form:
// separate forms cannot go back; conditional steps use WithHideFunc.
func NewForm(groups ...*huh.Group) *huh.Form {
	warmUp()
	submitting.Store(false) // Init validates on focus, before the filter runs
	// WithProgramOptions assigns teaOptions (dropping huh's defaults, hence
	// WithReportFocus); WithInput/WithOutput append to it.
	return huh.NewForm(groups...).
		WithProgramOptions(tea.WithReportFocus(), tea.WithFilter(armSubmit)).
		WithInput(os.Stdin).
		WithOutput(os.Stderr).
		WithTheme(theme()).
		WithKeyMap(keyMap())
}

// Run renders build's form at a safe height: ctrl+c returns a silent
// ErrCanceled, any other failure an internal error. build must construct fresh
// groups on every call — it may be called twice.
func Run(build func() *huh.Form) error {
	if err := sized(build).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return output.ErrCanceled()
		}
		return output.ErrInternal("interactive prompt failed: %v", err)
	}
	return nil
}

// sized picks the form height. The frame must never be as tall as the terminal:
// the renderer's trailing newline would scroll the title off screen.
func sized(build func() *huh.Form) *huh.Form {
	w, h := termSize()
	return sizedFor(build, w, h)
}

// sizedFor is sized with the terminal size injected. h <= 0 means "not a
// terminal", leaving the height to huh.
func sizedFor(build func() *huh.Form, w, h int) *huh.Form {
	if h <= 0 || neededRows(build, w)+1 <= h {
		return build()
	}
	return build().WithHeight(h - 2)
}

// neededRows reports how many rows build's form wants, by rendering it once
// against a very tall terminal. w must be the real terminal width.
func neededRows(build func() *huh.Form, w int) int {
	f := build()
	f.Init()
	m, _ := f.Update(tea.WindowSizeMsg{Width: w, Height: 9999})
	return len(strings.Split(strings.TrimRight(m.(*huh.Form).View(), "\n"), "\n"))
}

// termSize reports the size of the stderr terminal, or 0, 0 if it is not one.
func termSize() (int, int) {
	w, h, err := term.GetSize(os.Stderr.Fd())
	if err != nil {
		return 0, 0
	}
	return w, h
}

// submitting is true only while huh handles a key that leaves a field forwards.
// armSubmit maintains it; OnlyOnSubmit reads it. One flag is enough: Form.Run
// blocks, so a command never has two forms open.
var submitting atomic.Bool

// submitBindings are the keys that mean "leave this field forwards" — enter/tab
// for every huh v1.0.0 field type. Built on first use.
var submitBindings = sync.OnceValue(func() []key.Binding {
	km := keyMap()
	return []key.Binding{km.Input.Next, km.Input.Submit}
})

// armSubmit is the bubbletea message filter NewForm installs. Every message
// resets the flag, so it is set for exactly one Update.
func armSubmit(_ tea.Model, msg tea.Msg) tea.Msg {
	k, ok := msg.(tea.KeyMsg)
	submitting.Store(ok && key.Matches(k, submitBindings()...))
	return msg
}

// OnlyOnSubmit wraps a field validator so it only runs on the key that leaves
// the field forwards; huh otherwise validates on focus and refuses to leave a
// field holding an error. Armed only by NewForm's filter: on any other form it
// passes everything, so the command must re-check after Run.
func OnlyOnSubmit[T any](check func(T) error) func(T) error {
	return func(v T) error {
		if !submitting.Load() {
			return nil
		}
		return check(v)
	}
}
