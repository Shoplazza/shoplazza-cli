package themecmd

import (
	"context"
	"errors"
	"testing"
	"time"
)

// awaitServeStop is what makes `themes serve` block, so each way out of it is
// worth pinning: --timeout must end the watch on its own, a dead watcher must
// surface, and a zero timeout must never fire.
func TestAwaitServeStop(t *testing.T) {
	t.Run("timeout ends the watch cleanly", func(t *testing.T) {
		if err := awaitServeStop(context.Background(), make(chan error), 20*time.Millisecond); err != nil {
			t.Errorf("a bounded watch exits without an error, got %v", err)
		}
	})

	t.Run("a dead watcher is fatal", func(t *testing.T) {
		ch := make(chan error, 1)
		ch <- errors.New("inotify limit reached")
		err := awaitServeStop(context.Background(), ch, time.Minute)
		if err == nil {
			t.Fatal("a watcher failure must not look like a clean stop")
		}
	})

	t.Run("a canceled context stops without an error", func(t *testing.T) {
		ctx, cancel := context.WithCancel(context.Background())
		cancel()
		if err := awaitServeStop(ctx, make(chan error), 0); err != nil {
			t.Errorf("cancel is a clean stop, got %v", err)
		}
	})

	t.Run("a zero timeout never fires on its own", func(t *testing.T) {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Millisecond)
		defer cancel()
		start := time.Now()
		if err := awaitServeStop(ctx, make(chan error), 0); err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if elapsed := time.Since(start); elapsed < 25*time.Millisecond {
			t.Errorf("returned after %v — a zero timeout must wait for another signal", elapsed)
		}
	})
}
