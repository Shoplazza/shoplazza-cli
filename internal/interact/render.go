package interact

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for the output a command prints outside the form. Built on call, not
// at init: the palette is only valid after warmUp.

// Dim renders secondary text: labels, placeholders, hints.
func Dim(s string) string { warmUp(); return lipgloss.NewStyle().Foreground(brandMuted).Render(s) }

// cardStyle is the rounded box the closing summary sits in.
func cardStyle() lipgloss.Style {
	warmUp()
	return lipgloss.NewStyle().
		Border(lipgloss.RoundedBorder()).
		BorderForeground(brandMuted).
		Padding(0, 2)
}

// labelWidth is the column the summary card's values line up on.
const labelWidth = 10

// Field renders one "label   value" row of a summary card.
func Field(label, value string) string {
	return Dim(fmt.Sprintf("%-*s", labelWidth, label)) + " " + value
}

// FieldList renders a label with a comma-separated list wrapped to width.
// Continuation lines align under the value.
func FieldList(label string, items []string, width int) string {
	if len(items) == 0 {
		return Field(label, Dim("(none)"))
	}
	var lines []string
	cur := ""
	for i, it := range items {
		piece := it
		if i < len(items)-1 {
			piece += ","
		}
		switch {
		case cur == "":
			cur = piece
		case len(cur)+1+len(piece) <= width:
			cur += " " + piece
		default:
			lines = append(lines, cur)
			cur = piece
		}
	}
	lines = append(lines, cur)

	out := []string{Field(label, lines[0])}
	for _, l := range lines[1:] {
		out = append(out, Field("", l))
	}
	return strings.Join(out, "\n")
}

// Summary prints the closing card. Human-facing, so it draws on errOut.
func Summary(errOut io.Writer, rows ...string) {
	fmt.Fprintln(errOut, "\n"+cardStyle().Render(strings.Join(rows, "\n")))
}
