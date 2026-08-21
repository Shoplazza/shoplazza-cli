// Package interact holds the shared interaction layer: the brand huh theme, the
// form plumbing (stderr output, esc-goes-back, the height rule) and styles for
// output the form does not draw itself. Whether to prompt at all is decided by
// internal/output, not here.
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
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// Fixed brand colors: no light/dark variant, so no probe needed.
var (
	brandRed      = lipgloss.Color("#FF0000")
	brandIceBlue  = lipgloss.Color("#C7E3FF")
	brandPeach    = lipgloss.Color("#FFA180")
	brandCream    = lipgloss.Color("#F2EADA")
	brandDarkGray = lipgloss.Color("#4C5D66")
)

// Colors that depend on the terminal background, frozen by warmUp.
var (
	warm sync.Once

	// brandMuted is the dim color for secondary text.
	brandMuted lipgloss.Color

	// brandTitle is the softened red for headings.
	brandTitle lipgloss.Color
)

// warmUp freezes the palette against the terminal, once, before anything draws.
// Every accessor calls it, so it cannot be skipped. It queries the terminal, so
// it must not run at init.
func warmUp() { warm.Do(freeze) }

func freeze() {
	// Probes the default renderer too: huh.ThemeCharm's own AdaptiveColors
	// resolve against it, and an unprobed query fires mid-frame.
	_ = lipgloss.HasDarkBackground()
	stderrRenderer := lipgloss.NewRenderer(os.Stderr)
	dark := stderrRenderer.HasDarkBackground()

	// The default renderer reads stdout; forms draw on stderr.
	lipgloss.SetDefaultRenderer(stderrRenderer)

	pick := func(light, darkVal string) lipgloss.Color {
		if dark {
			return lipgloss.Color(darkVal)
		}
		return lipgloss.Color(light)
	}
	brandMuted = pick("#4C5D66", "#78888F")
	brandTitle = pick("#C62828", "#FF7A7A")
}

// theme recolors huh's Charm theme with the brand palette.
func theme() *huh.Theme {
	warmUp()
	t := huh.ThemeCharm()

	t.Focused.Title = t.Focused.Title.Foreground(brandTitle).Bold(true)
	t.Focused.NoteTitle = t.Focused.NoteTitle.Foreground(brandTitle).Bold(true)
	t.Focused.Description = t.Focused.Description.Foreground(brandMuted)
	t.Focused.SelectSelector = t.Focused.SelectSelector.Foreground(brandRed)
	t.Focused.MultiSelectSelector = t.Focused.MultiSelectSelector.Foreground(brandRed)
	t.Focused.NextIndicator = t.Focused.NextIndicator.Foreground(brandRed)
	t.Focused.PrevIndicator = t.Focused.PrevIndicator.Foreground(brandRed)
	t.Focused.SelectedOption = t.Focused.SelectedOption.Foreground(brandPeach)
	t.Focused.SelectedPrefix = lipgloss.NewStyle().Foreground(brandPeach).SetString("✓ ")
	t.Focused.UnselectedPrefix = lipgloss.NewStyle().Foreground(brandMuted).SetString("• ")
	t.Focused.FocusedButton = t.Focused.FocusedButton.Foreground(brandCream).Background(brandRed)
	t.Focused.BlurredButton = t.Focused.BlurredButton.Foreground(brandDarkGray).Background(brandIceBlue)
	t.Focused.Next = t.Focused.FocusedButton
	t.Focused.ErrorIndicator = t.Focused.ErrorIndicator.Foreground(brandRed)
	t.Focused.ErrorMessage = t.Focused.ErrorMessage.Foreground(brandRed)
	t.Focused.TextInput.Prompt = t.Focused.TextInput.Prompt.Foreground(brandRed)
	t.Focused.TextInput.Cursor = t.Focused.TextInput.Cursor.Foreground(brandPeach)
	t.Focused.TextInput.Placeholder = t.Focused.TextInput.Placeholder.Foreground(brandMuted)

	t.Blurred = t.Focused
	t.Blurred.Base = t.Focused.Base.BorderStyle(lipgloss.HiddenBorder())
	t.Blurred.Card = t.Blurred.Base
	t.Blurred.NextIndicator = lipgloss.NewStyle()
	t.Blurred.PrevIndicator = lipgloss.NewStyle()

	t.Group.Title = t.Focused.Title
	t.Group.Description = t.Focused.Description
	return t
}

// keyMap adds esc alongside shift+tab for "go back a step".
func keyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	back := key.NewBinding(key.WithKeys("shift+tab", "esc"), key.WithHelp("esc", "back"))
	km.Input.Prev = back
	km.Select.Prev = back
	km.MultiSelect.Prev = back
	return km
}

// NewForm wires the plumbing every interactive command needs: keystrokes from
// stdin, the UI on stderr (never stdout, which carries the result envelope),
// the brand theme, esc-goes-back. Pass every step as a group of ONE form:
// separate forms cannot go back; conditional steps use WithHideFunc.
func NewForm(groups ...*huh.Group) *huh.Form {
	warmUp()
	// WithProgramOptions assigns teaOptions; WithInput/WithOutput append to it,
	// so the filter has to be installed first or it replaces their options.
	return huh.NewForm(groups...).
		WithProgramOptions(tea.WithFilter(armSubmit)).
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

// submitBindings are the keys that mean "leave this field forwards", taken from
// the keymap so they track huh's defaults. Built on first use: a non-interactive
// run must not pay for it.
var submitBindings = sync.OnceValue(func() []key.Binding {
	km := keyMap()
	return []key.Binding{
		km.Input.Next, km.Input.Submit,
		km.Select.Next, km.Select.Submit,
		km.MultiSelect.Next, km.MultiSelect.Submit,
	}
})

// armSubmit is the bubbletea message filter NewForm installs. Every message
// resets the flag, so it is set for exactly one Update.
func armSubmit(_ tea.Model, msg tea.Msg) tea.Msg {
	k, ok := msg.(tea.KeyMsg)
	submitting.Store(ok && key.Matches(k, submitBindings()...))
	return msg
}

// OnlyOnSubmit wraps a field validator so it reports an error only on the key
// that submits the field. Select and MultiSelect otherwise validate on focus,
// on every toggle and on going back — and huh refuses to leave a field holding
// an error, which strands the user on the screen with no way back.
func OnlyOnSubmit[T any](check func(T) error) func(T) error {
	return func(v T) error {
		if !submitting.Load() {
			return nil
		}
		return check(v)
	}
}
