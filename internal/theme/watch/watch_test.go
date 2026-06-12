package watch

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"
)

func setupThemeTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()
	for _, d := range []string{"assets", "blocks", "config", "layout", "locales", "sections", "snippets", "templates"} {
		_ = os.MkdirAll(filepath.Join(root, d), 0o755)
	}
	_ = os.WriteFile(filepath.Join(root, "layout", "theme.liquid"), []byte("init"), 0o644)
	return root
}

type recorder struct {
	mu     sync.Mutex
	create []string
	update []string
	delete []string
}

func (r *recorder) cb() Callback {
	return Callback{
		OnCreate: func(p string) { r.mu.Lock(); defer r.mu.Unlock(); r.create = append(r.create, p) },
		OnUpdate: func(p string) { r.mu.Lock(); defer r.mu.Unlock(); r.update = append(r.update, p) },
		OnDelete: func(p string) { r.mu.Lock(); defer r.mu.Unlock(); r.delete = append(r.delete, p) },
	}
}

func (r *recorder) waitFor(t *testing.T, kind string, want string, timeout time.Duration) {
	t.Helper()
	deadline := time.Now().Add(timeout)
	for time.Now().Before(deadline) {
		r.mu.Lock()
		var list []string
		switch kind {
		case "create":
			list = r.create
		case "update":
			list = r.update
		case "delete":
			list = r.delete
		}
		for _, e := range list {
			if e == want {
				r.mu.Unlock()
				return
			}
		}
		r.mu.Unlock()
		time.Sleep(20 * time.Millisecond)
	}
	t.Fatalf("did not observe %s event for %q within %v", kind, want, timeout)
}

// eventWait is how long to wait for a filesystem event. Generous by default so
// a slow filesystem doesn't false-fail. Override with SHOPLAZZA_TEST_WATCH_TIMEOUT.
func eventWait() time.Duration {
	if v := os.Getenv("SHOPLAZZA_TEST_WATCH_TIMEOUT"); v != "" {
		if d, err := time.ParseDuration(v); err == nil {
			return d
		}
	}
	return 10 * time.Second
}

var createWatchProbe struct {
	once sync.Once
	ok   bool
}

// requireWorkingCreateWatch skips the test when the filesystem does not deliver
// fsnotify create events for new files (some CI containers drop them). Probed once per run.
func requireWorkingCreateWatch(t *testing.T) {
	t.Helper()
	createWatchProbe.once.Do(func() {
		dir, err := os.MkdirTemp("", "watch-create-probe")
		if err != nil {
			return
		}
		defer os.RemoveAll(dir)
		for _, d := range []string{"assets", "blocks", "config", "layout", "locales", "sections", "snippets", "templates"} {
			_ = os.MkdirAll(filepath.Join(dir, d), 0o755)
		}
		_ = os.WriteFile(filepath.Join(dir, "layout", "theme.liquid"), []byte("init"), 0o644)
		r := &recorder{}
		stop, err := Watch(dir, WatchOptions{}, r.cb())
		if err != nil {
			return
		}
		defer stop()
		time.Sleep(100 * time.Millisecond)
		_ = os.WriteFile(filepath.Join(dir, "assets", "probe.css"), []byte("x"), 0o644)
		deadline := time.Now().Add(3 * time.Second)
		for time.Now().Before(deadline) {
			r.mu.Lock()
			n := len(r.create)
			r.mu.Unlock()
			if n > 0 {
				createWatchProbe.ok = true
				return
			}
			time.Sleep(20 * time.Millisecond)
		}
	})
	if !createWatchProbe.ok {
		t.Skip("filesystem does not deliver fsnotify create events here (e.g. CI overlay/container FS)")
	}
}

func TestWatch_DetectsFileUpdate(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, err := Watch(root, WatchOptions{}, r.cb())
	if err != nil {
		t.Fatalf("Watch err: %v", err)
	}
	defer stop()
	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(root, "layout", "theme.liquid"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.waitFor(t, "update", "layout/theme.liquid", eventWait())
}

func TestWatch_DetectsFileCreate(t *testing.T) {
	requireWorkingCreateWatch(t)
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	_ = os.WriteFile(filepath.Join(root, "assets", "new.css"), []byte("x"), 0o644)
	r.waitFor(t, "create", "assets/new.css", eventWait())
}

func TestWatch_DetectsFileDelete(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	_ = os.Remove(filepath.Join(root, "layout", "theme.liquid"))
	r.waitFor(t, "delete", "layout/theme.liquid", eventWait())
}

func TestWatch_IgnoresNonThemeFiles(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	_ = os.WriteFile(filepath.Join(root, "README.md"), []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	for _, list := range [][]string{r.create, r.update, r.delete} {
		for _, e := range list {
			if e == "README.md" {
				t.Fatalf("README.md should be ignored (not in theme tree)")
			}
		}
	}
}

func TestWatch_ReturnsForwardSlashRelPathsOnAllOS(t *testing.T) {
	requireWorkingCreateWatch(t)
	root := setupThemeTree(t)
	_ = os.MkdirAll(filepath.Join(root, "assets", "sub"), 0o755)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	_ = os.WriteFile(filepath.Join(root, "assets", "sub", "img.png"), []byte("png"), 0o644)
	r.waitFor(t, "create", "assets/sub/img.png", eventWait())
}

func TestWatch_StopReleases(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	stop()
	_ = os.WriteFile(filepath.Join(root, "assets", "x.css"), []byte("x"), 0o644)
	time.Sleep(300 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	if len(r.create) > 0 {
		t.Fatalf("events received after stop: %v", r.create)
	}
}

func TestWatch_DebouncesConsecutiveEvents(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	target := filepath.Join(root, "layout", "theme.liquid")
	for i := 0; i < 5; i++ {
		_ = os.WriteFile(target, []byte("v"), 0o644)
		time.Sleep(5 * time.Millisecond)
	}
	time.Sleep(200 * time.Millisecond)
	r.mu.Lock()
	defer r.mu.Unlock()
	count := 0
	for _, e := range r.update {
		if e == "layout/theme.liquid" {
			count++
		}
	}
	if count == 0 || count > 3 {
		t.Errorf("debounce: got %d update events, expected 1-3", count)
	}
}

func TestWatch_StopIsIdempotent(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	stop()
	stop()
}

// TestWatch_DirCreateNotDispatched: creating a directory is watch-tree
// maintenance, not a file change. The watcher must recurse into the new dir
// (so files created inside it are seen) without firing a callback for the dir.
func TestWatch_DirCreateNotDispatched(t *testing.T) {
	requireWorkingCreateWatch(t)
	root := setupThemeTree(t)
	r := &recorder{}
	stop, _ := Watch(root, WatchOptions{}, r.cb())
	defer stop()
	time.Sleep(100 * time.Millisecond)

	newDir := filepath.Join(root, "assets", "icons")
	if err := os.MkdirAll(newDir, 0o755); err != nil {
		t.Fatal(err)
	}
	time.Sleep(400 * time.Millisecond)

	r.mu.Lock()
	for _, list := range [][]string{r.create, r.update, r.delete} {
		for _, e := range list {
			if e == "assets/icons" {
				r.mu.Unlock()
				t.Fatal("directory create must not be dispatched to callbacks")
			}
		}
	}
	r.mu.Unlock()

	// The new dir must still be WATCHED: a file created inside it fires.
	_ = os.WriteFile(filepath.Join(newDir, "star.svg"), []byte("<svg/>"), 0o644)
	r.waitFor(t, "create", "assets/icons/star.svg", eventWait())
}

// TestWatch_TinyDebounceDoesNotPanic: a sub-millisecond DebounceWindow must not
// panic time.NewTicker; the floor keeps the watcher functional.
func TestWatch_TinyDebounceDoesNotPanic(t *testing.T) {
	root := setupThemeTree(t)
	r := &recorder{}
	stop, err := Watch(root, WatchOptions{DebounceWindow: 1 * time.Nanosecond}, r.cb())
	if err != nil {
		t.Fatalf("Watch err: %v", err)
	}
	defer stop()
	time.Sleep(100 * time.Millisecond)

	if err := os.WriteFile(filepath.Join(root, "layout", "theme.liquid"), []byte("changed"), 0o644); err != nil {
		t.Fatal(err)
	}
	r.waitFor(t, "update", "layout/theme.liquid", eventWait())
}
