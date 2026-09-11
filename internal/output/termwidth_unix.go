//go:build !windows

package output

import (
	"io"
	"os"

	"golang.org/x/sys/unix"
)

// terminalWidth returns the column count of the terminal behind w, or 0 when unknown.
func terminalWidth(w io.Writer) int {
	f, ok := w.(*os.File)
	if !ok {
		return 0
	}
	ws, err := unix.IoctlGetWinsize(int(f.Fd()), unix.TIOCGWINSZ)
	if err != nil || ws.Col == 0 {
		return 0
	}
	return int(ws.Col)
}
