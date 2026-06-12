package output

import (
	"bytes"
	"strings"
	"testing"
	"time"
)

// A nil *Progress and the *Step it yields must be safe no-ops so callers
// (javy path, tests) can opt out without branching.
func TestProgress_NilIsNoOp(t *testing.T) {
	var p *Progress
	s := p.Begin("anything")
	s.Done() // must not panic
	s.Fail() // must not panic
}

// Non-terminal writers (bytes.Buffer here, like a piped/CI stderr) must get one
// clean static line per step with no carriage returns or ANSI escapes — so
// captured output is never corrupted.
func TestProgress_NonTTY_StaticLine(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	if p.isTTY {
		t.Fatal("bytes.Buffer must be treated as non-terminal")
	}

	p.Begin("Starting cloudflared tunnel").Done()
	p.Begin("Downloading cloudflared").Fail()

	out := buf.String()
	if strings.ContainsAny(out, "\r") || strings.Contains(out, "\033[") {
		t.Fatalf("non-TTY output must contain no \\r or ANSI escapes, got %q", out)
	}
	lines := strings.Split(strings.TrimRight(out, "\n"), "\n")
	if len(lines) != 2 {
		t.Fatalf("want 2 lines, got %d: %q", len(lines), out)
	}
	if !strings.HasPrefix(lines[0], "Starting cloudflared tunnel... ") {
		t.Errorf("line 0 = %q", lines[0])
	}
	if !strings.Contains(lines[1], "failed") {
		t.Errorf("failed step must say so: %q", lines[1])
	}
}

// Done is idempotent: a second call (or a Fail after Done) must not emit a
// second line or panic.
func TestProgress_FinishOnce(t *testing.T) {
	var buf bytes.Buffer
	p := NewProgress(&buf)
	s := p.Begin("step")
	s.Done()
	s.Done()
	s.Fail()
	if got := strings.Count(buf.String(), "\n"); got != 1 {
		t.Fatalf("want exactly 1 finalized line, got %d: %q", got, buf.String())
	}
}

func TestFmtElapsed(t *testing.T) {
	cases := []struct {
		d    time.Duration
		want string
	}{
		{1200 * time.Millisecond, "1.2s"},
		{500 * time.Millisecond, "0.5s"},
		{59900 * time.Millisecond, "59.9s"},
		{63 * time.Second, "1m03s"},
		{125 * time.Second, "2m05s"},
	}
	for _, c := range cases {
		if got := fmtElapsed(c.d); got != c.want {
			t.Errorf("fmtElapsed(%s) = %q, want %q", c.d, got, c.want)
		}
	}
}
