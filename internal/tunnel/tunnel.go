// Package tunnel exposes a local dev port over a public HTTPS URL for `app dev`.
//
// A list of strategies is tried in order and the first one that comes up wins;
// if every strategy fails their errors are joined into a single network-class
// failure. cloudflared (quick tunnel) is the primary strategy and ngrok the
// fallback.
package tunnel

import (
	"context"
	"os"
	"runtime"
	"strings"
	"syscall"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// gracefulKill sends SIGTERM on Unix and falls back to Process.Kill on Windows
// where SIGTERM is a no-op ("not supported by windows").
func gracefulKill(p *os.Process) error {
	if runtime.GOOS == "windows" {
		return p.Kill()
	}
	return p.Signal(syscall.SIGTERM)
}

// Tunnel is a live public endpoint forwarding to the local dev port. Close stops
// the underlying provider process.
type Tunnel struct {
	URL   string
	Close func() error
}

// Strategy is one way to obtain a public tunnel (cloudflared, ngrok, ...).
type Strategy interface {
	// Name identifies the strategy in log/error output.
	Name() string
	// Start brings up a tunnel to http://localhost:<port>, blocking until the
	// public URL is known (or it fails / times out).
	Start(ctx context.Context, port int) (*Tunnel, error)
}

// Default returns the production strategy order: cloudflared first, ngrok as the
// fallback.
func Default() []Strategy {
	return []Strategy{&Cloudflared{}, &Ngrok{}}
}

// Open tries each strategy in order and returns the first tunnel that comes up.
// If every strategy fails, their errors are joined into a single network-class
// ExitError.
func Open(ctx context.Context, port int, strategies ...Strategy) (*Tunnel, *output.ExitError) {
	var failures []string
	for _, s := range strategies {
		t, err := s.Start(ctx, port)
		if err == nil {
			return t, nil
		}
		failures = append(failures, s.Name()+": "+err.Error())
	}
	return nil, output.ErrNetwork("all tunnel strategies failed: %s", strings.Join(failures, "; "))
}
