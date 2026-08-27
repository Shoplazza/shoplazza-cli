// Package interact holds the shared interaction layer: the brand huh theme, the
// form plumbing (stderr output, esc-goes-back, the height rule) and styles for
// output the form does not draw itself. Whether to prompt at all is decided by
// internal/output, not here.
package interact

import (
	"os"
	"sync"

	"github.com/charmbracelet/bubbles/key"
	"github.com/charmbracelet/huh"
	"github.com/charmbracelet/lipgloss"
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
