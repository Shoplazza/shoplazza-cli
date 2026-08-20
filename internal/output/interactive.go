package output

import (
	"os"

	"github.com/charmbracelet/x/term"
)

// interactiveOffEnv are the escape hatches that close the gate. Any of them set
// to a non-empty value is enough; the value is never compared, since CI systems
// variously use CI=true, CI=1 or CI=yes. Missing one hangs a build, a false
// positive only costs a prompt.
var interactiveOffEnv = []string{"SHOPLAZZA_CLI_NO_INTERACTIVE", "CI"}

// IsTerminal reports whether v is backed by a character device (a real
// terminal). It takes any so one implementation serves both writers (Progress)
// and *os.File streams (the gate below): values with no Stat method, such as a
// bytes.Buffer, are never terminals, and neither are files that are not
// character devices (pipes, redirects). Stdlib only — the interaction package
// pulls in x/term for terminal SIZE, but this check needs nothing beyond Stat.
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

// Interactive reports whether a human is present to answer prompts: in must be
// readable as keystrokes and errOut drawable as a UI. stdout is deliberately
// excluded — it may be piped to jq while the prompt still renders on stderr.
// env is injected (os.LookupEnv in production) so the gate is unit-testable
// without a terminal. There is only an off switch by design; forcing it on
// would let a headless run hang forever.
func Interactive(in, errOut *os.File, env func(string) (string, bool)) bool {
	return interactive(env, func() bool { return IsTTY(in) && IsTTY(errOut) })
}

// interactive is the gate with the terminal question injected, so the
// "a terminal IS present but an escape hatch is set" direction is testable —
// go test has no tty to hand, and that direction is the one that hangs builds.
func interactive(env func(string) (string, bool), bothTTY func() bool) bool {
	for _, k := range interactiveOffEnv {
		if v, ok := env(k); ok && v != "" {
			return false
		}
	}
	return bothTTY()
}

// IsTTY reports whether f is a real terminal — not merely a character device.
//
// The distinction is load-bearing, and IsTerminal is NOT a substitute here.
// os.ModeCharDevice is true for /dev/null and /dev/zero too, so a run redirected
// with `< /dev/null 2>/dev/null` — a very ordinary shape for CI and agents —
// passed the char-device test, opened a prompt against /dev/null and waited
// forever for a keystroke. Measured: `app init` under that redirection hung,
// and exited immediately with the escape hatch set.
//
// IsTerminal answers "can I draw here", where a character device is enough and
// being wrong only writes to the void. This answers "is a human here", where
// being wrong blocks the process. Two questions, two predicates.
func IsTTY(f *os.File) bool {
	return f != nil && term.IsTerminal(f.Fd())
}
