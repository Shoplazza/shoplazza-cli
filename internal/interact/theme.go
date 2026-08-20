// Package interact holds the one shared interaction layer: the brand huh theme,
// the form plumbing (stderr output, esc-goes-back, the height rule) and the
// styles for output the form does not draw itself. There is deliberately only
// one copy — a command carrying its own palette would drift from the rest.
//
// The TTY gate is NOT here: it lives in internal/output next to the exit codes,
// and this package never decides whether to prompt, only how it looks.
package interact

import (
	"errors"
	"os"
	"strings"
	"sync"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"

	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/term"
)

// Fixed brand colors — no light/dark variant, so no probe needed. The pure
// brand red is spent only on small high-signal marks (cursor, error text).
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

	// brandMuted: secondary text must actually recede. Ice blue is
	// high-luminance and shouts on a dark terminal, so the dark variant is a
	// lightened brand gray instead.
	brandMuted lipgloss.Color

	// brandTitle: pure #FF0000 is punishing across a whole line of bold text,
	// so headings get a softened red.
	brandTitle lipgloss.Color
)

// warmUp freezes the palette against the terminal, once, before anything draws.
//
// Why not lipgloss.AdaptiveColor: it resolves lazily, at Render time, sending
// the terminal an OSC-11 background query plus a cursor position report —
// bytes that can land mid-frame and corrupt it.
//
// Why not package init: cmd/auth is linked into every invocation, so a probe at
// init would make `... | jq` and every unrelated command pay for a terminal
// query they never use. Interactive commands call this on the interactive path
// only; every accessor below calls it too, so it can never be skipped.
func warmUp() { warm.Do(freeze) }

func freeze() {
	// Probe BOTH channels. Our own colors are frozen against stderr (where
	// forms draw), but huh.ThemeCharm carries AdaptiveColors of its own that
	// resolve against the default renderer — leave that unprobed and the query
	// still fires mid-frame.
	_ = lipgloss.HasDarkBackground()
	stderrRenderer := lipgloss.NewRenderer(os.Stderr)
	dark := stderrRenderer.HasDarkBackground()

	// Styles must take their color profile from the stream they are drawn on.
	// lipgloss's default renderer reads stdout, so under `auth login | jq .`
	// the whole UI — huh's own theme included — comes out unstyled even though
	// the terminal it draws on supports color.
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

// theme recolors Charm with the brand palette. The cursor stays brand red; the
// SELECTED row goes peach rather than a second red, so "where I am" and "what I
// picked" never blur together.
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
//
// esc is safe to overload: huh enables its filter bindings conditionally
// (SetFilter only while filtering, ClearFilter only when a stale filter
// remains), disables Prev while filtering, and matches those cases earlier. So
// esc escalates — exit filter, then clear filter, then go back a step.
func keyMap() *huh.KeyMap {
	km := huh.NewDefaultKeyMap()
	back := key.NewBinding(key.WithKeys("shift+tab", "esc"), key.WithHelp("esc", "back"))
	km.Input.Prev = back
	km.Select.Prev = back
	km.MultiSelect.Prev = back
	return km
}

// NewForm wires the plumbing every interactive command needs: keystrokes from
// stdin, the UI on stderr, the brand theme, esc-goes-back.
//
// Drawing on stderr is mandatory, not cosmetic: a form is a stream of ANSI
// sequences, and on stdout `shoplazza auth login | jq .` would feed them to jq
// along with the result envelope. WithOutput also points bubbletea's size
// detection at stderr.
//
// Pass every step as a group of ONE form. Separate forms per step cannot go
// back; conditional steps use WithHideFunc instead.
func NewForm(groups ...*huh.Group) *huh.Form {
	warmUp()
	return huh.NewForm(groups...).
		WithInput(os.Stdin).
		WithOutput(os.Stderr).
		WithTheme(theme()).
		WithKeyMap(keyMap())
}

// Run renders build's form at a safe height and translates the outcome:
// ctrl+c becomes a silent exit-130 error (nil Detail, so no envelope and no
// message reaches either stream — the exit code says it all), anything else an
// internal error. build must construct fresh groups on each call: Sized may
// call it twice.
func Run(build func() *huh.Form) error {
	if err := sized(build).Run(); err != nil {
		if errors.Is(err, huh.ErrUserAborted) {
			return output.ErrCanceled()
		}
		return output.ErrInternal("interactive prompt failed: %v", err)
	}
	return nil
}

// sized enforces the one hard rule about form height: the frame must never be
// as tall as the terminal, or the renderer's trailing newline scrolls the first
// line — the title — off screen.
//
// huh's own default is min(content, terminal): correct and tight until the
// content does not fit, at which point it saturates at the full terminal height
// and the title is lost. So leave it alone when it fits, cap it when it does
// not. No estimate, no magic constant — a fixed WithHeight is worse than
// nothing on a short terminal.
func sized(build func() *huh.Form) *huh.Form {
	w, h := termSize()
	return sizedFor(build, w, h)
}

// sizedFor is the decision, split out from the terminal read so it can be
// tested: pass a synthetic terminal size and assert the frame never fills it.
// h <= 0 means "not a terminal", where sizing is huh's business.
func sizedFor(build func() *huh.Form, w, h int) *huh.Form {
	if h <= 0 || neededRows(build, w)+1 <= h {
		return build()
	}
	return build().WithHeight(h - 2)
}

// neededRows asks huh itself how tall a form wants to be: render it once, offline,
// against an absurdly TALL terminal and count the lines.
//
// The WIDTH must be the real one — lipgloss pads every line out to it, so a
// 9999-wide probe costs 3.5x the time and twice the memory for the same answer.
//
// Verified: the probe does not write back into the bound variables (huh only
// does that on field submit), so passing the real builder is safe.
func neededRows(build func() *huh.Form, w int) int {
	f := build()
	f.Init()
	m, _ := f.Update(tea.WindowSizeMsg{Width: w, Height: 9999})
	return len(strings.Split(strings.TrimRight(m.(*huh.Form).View(), "\n"), "\n"))
}

// termSize reports the stderr terminal's size, where forms draw. A zero height
// means "not a terminal", leaving sizing entirely to huh.
func termSize() (int, int) {
	w, h, err := term.GetSize(os.Stderr.Fd())
	if err != nil {
		return 0, 0
	}
	return w, h
}

// NotOnArrival wraps a field validator so it stays quiet until the user has
// actually done something.
//
// huh runs Validate once when the field takes focus, before any keypress, and
// it refuses to leave a group that holds an error. Together those two turn a
// plain constraint into a trap: the screen arrives already red, and esc is
// silently swallowed until the user satisfies a rule they have not yet had a
// chance to break. Measured in a pty: esc produced no frame at all.
//
// Skipping that first call gives the intended behaviour — nothing on arrival,
// the error on enter. The call pattern this relies on (exactly one call from
// Init) is pinned by TestHuhCallsValidateOnceOnFocus; if a huh upgrade changes
// it, that test fails rather than this quietly reverting to the trap.
//
// Residual, and inherent: emptying a selection back to nothing does raise the
// error, and esc is blocked while it stands. That follows a deliberate action
// and one keypress clears it, unlike the arrival case.
func NotOnArrival[T any](check func(T) error) func(T) error {
	arrived := false
	return func(v T) error {
		if !arrived {
			arrived = true
			return nil
		}
		return check(v)
	}
}
