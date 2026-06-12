package watch

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/fsnotify/fsnotify"

	"shoplazza-cli-v2/internal/theme/doc"
)

// WatchOptions controls the watcher behavior.
type WatchOptions struct {
	// Filter is an optional predicate; when nil, only paths ParseThemeFile
	// accepts are kept. The argument is the forward-slash relative path under srcDir.
	Filter         func(relPath string) bool
	DebounceWindow time.Duration // default 50ms
}

// Callback holds the event handlers. Each fires sequentially from one internal
// goroutine, so keep them quick or events may be dropped. The same relPath may
// fire 1-3 times within the debounce window on editor save-then-rename. OnError
// is optional and fires for fsnotify error-channel events; when nil they are dropped.
type Callback struct {
	OnCreate func(relPath string)
	OnUpdate func(relPath string)
	OnDelete func(relPath string)
	OnError  func(error)
}

// Watch starts an fsnotify-based recursive watcher on the 8 standard theme
// directories under srcDir. Returns a stop func; calling it stops the watcher
// and closes the fsnotify resources. stop() is idempotent.
func Watch(srcDir string, opts WatchOptions, cb Callback) (func(), error) {
	w, err := fsnotify.NewWatcher()
	if err != nil {
		return nil, err
	}
	for _, d := range []string{"assets", "blocks", "config", "layout", "locales", "sections", "snippets", "templates"} {
		full := filepath.Join(srcDir, d)
		if _, err := os.Stat(full); errors.Is(err, os.ErrNotExist) {
			continue
		}
		if err := addDirRecursive(w, full); err != nil {
			_ = w.Close()
			return nil, err
		}
	}

	debounce := opts.DebounceWindow
	if debounce <= 0 {
		debounce = 50 * time.Millisecond
	}
	// Floor: the dispatch ticker runs at debounce/2 and time.NewTicker panics
	// on a non-positive duration, so keep the tick at least 1ms.
	if debounce < 2*time.Millisecond {
		debounce = 2 * time.Millisecond
	}
	stopCh := make(chan struct{})
	var stopOnce sync.Once
	stop := func() {
		stopOnce.Do(func() {
			close(stopCh)
			_ = w.Close()
		})
	}

	type pending struct {
		op fsnotify.Op
		t  time.Time
	}
	mu := &sync.Mutex{}
	pendingMap := map[string]pending{}

	go func() {
		ticker := time.NewTicker(debounce / 2)
		defer ticker.Stop()
		for {
			select {
			case <-stopCh:
				return
			case ev, ok := <-w.Events:
				if !ok {
					return
				}
				if ev.Op&fsnotify.Create == fsnotify.Create {
					// A new directory is watch-tree maintenance only: recurse
					// into it but don't dispatch — a dir create is not a file change.
					if fi, err := os.Stat(ev.Name); err == nil && fi.IsDir() {
						if aerr := addDirRecursive(w, ev.Name); aerr != nil && cb.OnError != nil {
							cb.OnError(aerr)
						}
						continue
					}
				}
				rel, err := filepath.Rel(srcDir, ev.Name)
				if err != nil {
					continue
				}
				relSlash := filepath.ToSlash(rel)
				if opts.Filter != nil {
					if !opts.Filter(relSlash) {
						continue
					}
				} else {
					if _, _, err := doc.ParseThemeFile(relSlash); err != nil {
						continue
					}
				}
				mu.Lock()
				pendingMap[relSlash] = pending{op: ev.Op, t: time.Now()}
				mu.Unlock()
			case <-ticker.C:
				now := time.Now()
				mu.Lock()
				ready := make(map[string]pending)
				for k, v := range pendingMap {
					if now.Sub(v.t) >= debounce {
						ready[k] = v
						delete(pendingMap, k)
					}
				}
				mu.Unlock()
				for relSlash, p := range ready {
					switch {
					case p.op&fsnotify.Create == fsnotify.Create:
						if cb.OnCreate != nil {
							cb.OnCreate(relSlash)
						}
					case p.op&fsnotify.Remove == fsnotify.Remove || p.op&fsnotify.Rename == fsnotify.Rename:
						if cb.OnDelete != nil {
							cb.OnDelete(relSlash)
						}
					case p.op&fsnotify.Write == fsnotify.Write || p.op&fsnotify.Chmod == fsnotify.Chmod:
						if cb.OnUpdate != nil {
							cb.OnUpdate(relSlash)
						}
					}
				}
			case err, ok := <-w.Errors:
				if !ok {
					return
				}
				if cb.OnError != nil {
					cb.OnError(err)
				}
			}
		}
	}()
	return stop, nil
}

func addDirRecursive(w *fsnotify.Watcher, root string) error {
	return filepath.Walk(root, func(p string, info os.FileInfo, err error) error {
		if err != nil {
			return nil
		}
		if !info.IsDir() {
			return nil
		}
		base := filepath.Base(p)
		if strings.HasPrefix(base, ".") && p != root {
			return filepath.SkipDir
		}
		return w.Add(p)
	})
}
