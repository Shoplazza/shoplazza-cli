package output

import (
	"fmt"
	"io"
	"os"
	"sync"
	"time"
	"unicode/utf8"
)

// progressTick is how often a running step rewrites its elapsed-time line on a
// terminal.
const progressTick = 100 * time.Millisecond

// termWidth reports the terminal column count; tests replace it.
var termWidth = terminalWidth

// Progress renders step-by-step progress for long-running setup work. On a
// terminal each step shows a live elapsed timer that refreshes in place every
// 100ms and freezes on completion; off a terminal (piped, captured, CI) it
// degrades to one static line per step. Output goes to the provided writer
// (stderr, so stdout stays clean for the JSON envelope). A nil *Progress is a
// valid no-op receiver.
type Progress struct {
	w     io.Writer
	isTTY bool
	mu    sync.Mutex // serializes writes so concurrent steps never interleave
}

// NewProgress returns a reporter writing to w. It detects whether w is a
// terminal (an *os.File backed by a character device) to decide between live
// in-place refresh and plain static lines.
func NewProgress(w io.Writer) *Progress {
	return &Progress{w: w, isTTY: isTerminalWriter(w)}
}

// Step is one in-progress unit of work started by Progress.Begin. Finalize it
// exactly once with Done (success) or Fail (failure).
type Step struct {
	p     *Progress
	label string
	start time.Time
	stop  chan struct{} // closed to ask the ticker to exit (TTY only)
	done  chan struct{} // closed by the ticker once it has exited (TTY only)
	once  sync.Once     // guards finish so Done/Fail are safe to call more than once
	rows  int           // terminal rows the last frame wrapped onto (TTY only)
}

// Begin starts a step labeled label. On a terminal it launches a 100ms ticker
// that rewrites the line with the running elapsed time; otherwise it stays
// silent until the step is finalized. Call Done or Fail to finish it. A nil
// *Progress returns a nil *Step, which is itself a no-op.
func (p *Progress) Begin(label string) *Step {
	if p == nil {
		return nil
	}
	s := &Step{p: p, label: label, start: time.Now()}
	if p.isTTY {
		s.stop = make(chan struct{})
		s.done = make(chan struct{})
		go s.run()
	}
	return s
}

// run drives the live ticker on a terminal: render now, then once per tick until
// asked to stop.
func (s *Step) run() {
	defer close(s.done)
	t := time.NewTicker(progressTick)
	defer t.Stop()
	s.render()
	for {
		select {
		case <-s.stop:
			return
		case <-t.C:
			s.render()
		}
	}
}

// render redraws the frame with the live elapsed time (TTY only).
func (s *Step) render() {
	s.p.mu.Lock()
	defer s.p.mu.Unlock()
	s.redraw(fmt.Sprintf("%s... %s", s.label, fmtElapsed(time.Since(s.start))), false)
}

// redraw replaces the previous frame with text: the cursor moves up over any
// rows the previous frame wrapped onto, then the screen is cleared from there.
// A final frame ends the line and stops tracking rows.
func (s *Step) redraw(text string, final bool) {
	if s.rows > 1 {
		fmt.Fprintf(s.p.w, "\033[%dA", s.rows-1)
	}
	fmt.Fprintf(s.p.w, "\r\033[J%s", text)
	if final {
		fmt.Fprint(s.p.w, "\n")
		s.rows = 0
		return
	}
	s.rows = frameRows(text, termWidth(s.p.w))
}

// frameRows returns how many terminal rows text occupies at cols columns.
func frameRows(text string, cols int) int {
	n := utf8.RuneCountInString(text)
	if cols <= 0 || n <= cols {
		return 1
	}
	return (n + cols - 1) / cols
}

// Done finalizes the step as succeeded, freezing the line at its final elapsed
// time. Safe on a nil *Step and idempotent.
func (s *Step) Done() { s.finish("") }

// Fail finalizes the step as failed. Safe on a nil *Step and idempotent.
func (s *Step) Fail() { s.finish("failed") }

func (s *Step) finish(state string) {
	if s == nil {
		return
	}
	s.once.Do(func() {
		el := fmtElapsed(time.Since(s.start))
		if s.p.isTTY {
			// Stop the ticker and wait for it to exit before the final write, so
			// nothing races on the same line.
			close(s.stop)
			<-s.done
		}
		s.p.mu.Lock()
		defer s.p.mu.Unlock()
		text := fmt.Sprintf("%s... %s", s.label, el)
		if state != "" {
			text = fmt.Sprintf("%s... %s (%s)", s.label, state, el)
		}
		if s.p.isTTY {
			s.redraw(text, true)
		} else {
			fmt.Fprintln(s.p.w, text) // no ANSI on a non-terminal
		}
	})
}

// fmtElapsed formats a running duration compactly: "1.2s" under a minute,
// "1m03s" beyond.
func fmtElapsed(d time.Duration) string {
	if d < time.Minute {
		return fmt.Sprintf("%.1fs", d.Seconds())
	}
	m := int(d / time.Minute)
	s := int((d % time.Minute) / time.Second)
	return fmt.Sprintf("%dm%02ds", m, s)
}

// isTerminalWriter reports whether w is a character device (a real terminal).
// It avoids an x/term dependency by checking the file mode of any writer that
// exposes Stat (e.g. *os.File). Anything else (bytes.Buffer, pipes) is treated
// as non-terminal.
func isTerminalWriter(w io.Writer) bool {
	f, ok := w.(interface{ Stat() (os.FileInfo, error) })
	if !ok {
		return false
	}
	fi, err := f.Stat()
	if err != nil {
		return false
	}
	return fi.Mode()&os.ModeCharDevice != 0
}
