//go:build windows

package output

import "io"

// terminalWidth is unknown on Windows; frames are treated as single-row.
func terminalWidth(io.Writer) int { return 0 }
