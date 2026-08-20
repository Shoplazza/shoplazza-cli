package output

import "os"

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
	for _, k := range interactiveOffEnv {
		if v, ok := env(k); ok && v != "" {
			return false
		}
	}
	return IsTerminal(in) && IsTerminal(errOut)
}
