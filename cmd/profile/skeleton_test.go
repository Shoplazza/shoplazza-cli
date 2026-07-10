package profile

import (
	"errors"
	"testing"

	"shoplazza-cli-v2/internal/output"
)

// T11 fills in these command bodies; T10 only guarantees the package
// registers and compiles with the right flags and a typed "not implemented" error.
func TestSkeletons_NotImplemented(t *testing.T) {
	f := newTestFactory(t, "")
	for _, args := range [][]string{
		{"use", "--name", "us"},
		{"update", "--name", "us"},
		{"rename", "--name", "us", "--new-name", "usa"},
		{"remove", "--name", "us"},
	} {
		err := runCmdErr(t, f, args...)
		var exitErr *output.ExitError
		if !errors.As(err, &exitErr) || exitErr.Code != output.ExitInternal {
			t.Errorf("args=%v: err=%v, want ExitInternal", args, err)
		}
	}
}
