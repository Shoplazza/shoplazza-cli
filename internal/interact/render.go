package interact

import (
	"fmt"
	"io"
	"strings"

	"github.com/charmbracelet/lipgloss"
)

// Styles for the output a command prints OUTSIDE the form — the summary card,
// status lines, closing hints. They live here with the form theme so the two
// halves of one command's output cannot drift apart, nor two commands from each
// other. Built on call, not at init: the palette is only valid after WarmUp.

// Dim renders secondary text: labels, placeholders, hints.
func Dim(s string) string { WarmUp(); return lipgloss.NewStyle().Foreground(brandMuted).Render(s) }

// Bold renders text lifted by weight alone — for an option that ranks above its
// neighbours while staying left-aligned with them (no indent, no grey aside).
func Bold(s string) string { return lipgloss.NewStyle().Bold(true).Render(s) }

// cardStyle is the rounded box the closing summary sits in.
func cardStyle() lipgloss.Style {
	WarmUp()
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

// FieldList renders a label with a comma-separated list, wrapped to width so
// the card stays narrow. Continuation lines align under the value.
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

// Summary prints the "here is what you chose" card. Human-facing, so errOut —
// stdout carries the result envelope alone.
func Summary(errOut io.Writer, rows ...string) {
	fmt.Fprintln(errOut, "\n"+cardStyle().Render(strings.Join(rows, "\n")))
}

// Step prints one completed-phase line. The check mark reuses the selection
// accent rather than adding a fourth color family.
func Step(errOut io.Writer, msg string) {
	WarmUp()
	fmt.Fprintf(errOut, "%s %s\n", lipgloss.NewStyle().Foreground(brandPeach).Render("✓"), msg)
}

// Next prints the closing "what to run now" hint.
func Next(errOut io.Writer, cmd string) {
	fmt.Fprintf(errOut, "\n%s  %s\n\n", Dim("Next:"), cmd)
}
