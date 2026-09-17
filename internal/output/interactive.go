package output

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// interactiveOffEnv are the escape hatches that close the gate. Any one of them
// set to a non-empty value is enough; the value itself is never compared.
var interactiveOffEnv = []string{"SHOPLAZZA_CLI_NO_INTERACTIVE", "CI"}

// IsTerminal reports whether v is backed by a character device: false for
// anything without a Stat method (bytes.Buffer) and for pipes. It answers "can
// I draw here"; it is true for /dev/null too, so gate prompts on IsTTY instead.
func IsTerminal(v any) bool {
	f, ok := v.(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}

// minPromptCols is the narrowest terminal a prompt is drawn in: below it huh's
// layout math panics, so the gate treats the terminal as absent.
const minPromptCols = 8

// Interactive reports whether a human is present to answer prompts, on a
// terminal wide enough to draw one: in and errOut must both be real terminals
// with no escape hatch set. stdout is not consulted — it may be piped while the
// prompt draws on stderr. env is injected (os.LookupEnv in production). There is
// an off switch only, by design.
func Interactive(in, errOut *os.File, env func(string) (string, bool)) bool {
	return interactive(env, func() bool { return IsTTY(in) && IsTTY(errOut) }, func() int { return cols(errOut) })
}

// interactive is the gate with the terminal facts injected, so tests can cover
// every direction without a tty.
func interactive(env func(string) (string, bool), bothTTY func() bool, width func() int) bool {
	for _, k := range interactiveOffEnv {
		if v, ok := env(k); ok && v != "" {
			return false
		}
	}
	return bothTTY() && width() >= minPromptCols
}

// cols reports f's terminal width, or 0 when it has none.
func cols(f *os.File) int {
	if f == nil {
		return 0
	}
	w, _, err := term.GetSize(f.Fd())
	if err != nil {
		return 0
	}
	return w
}

// IsTTY reports whether f is a real terminal. Not interchangeable with
// IsTerminal, which is also true for /dev/null, where a prompt waits forever.
func IsTTY(f *os.File) bool {
	return f != nil && term.IsTerminal(f.Fd())
}
