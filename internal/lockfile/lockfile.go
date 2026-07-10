// Package lockfile wraps gofrs/flock with a TryLock+timeout loop.
// Locks release automatically on process exit (fd-based), so a crashed
// holder never leaves a stale lock.
package lockfile

import (
	"errors"
	"os"
	"path/filepath"
	"time"

	"github.com/gofrs/flock"
)

// ErrTimeout is returned when the lock is not acquired within the deadline.
var ErrTimeout = errors.New("lockfile: acquire timeout")

const retryInterval = 50 * time.Millisecond

// Acquire takes an exclusive cross-process lock on path, retrying every
// 50ms until timeout. The returned release func unlocks and is safe to
// call exactly once (use defer).
func Acquire(path string, timeout time.Duration) (func(), error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	fl := flock.New(path)
	deadline := time.Now().Add(timeout)
	for {
		ok, err := fl.TryLock()
		if err != nil {
			return nil, err
		}
		if ok {
			return func() { _ = fl.Unlock() }, nil
		}
		if time.Now().After(deadline) {
			return nil, ErrTimeout
		}
		time.Sleep(retryInterval)
	}
}
