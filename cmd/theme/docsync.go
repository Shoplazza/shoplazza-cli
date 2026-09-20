package themecmd

import (
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/doc"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/watch"
)

// buildWatchFilter returns serve's file filter: keep only real theme-tree files,
// skipping editor temp/swap/hidden artifacts and any path matched by
// .themeignore (nil ignorer → no ignore rules). Shared by the fsnotify watcher
// and the dedup-seeding walk so both apply identical rules.
func buildWatchFilter(ignorer pack.Ignorer) func(rel string) bool {
	return func(rel string) bool {
		if doc.IsEditorTemp(rel) {
			return false
		}
		if ignorer != nil && ignorer.MatchesPath(rel) {
			return false
		}
		_, _, perr := doc.ParseThemeFile(rel)
		return perr == nil
	}
}

// pendingFailures is a thread-safe set of relative file paths that failed their
// last sync attempt, so warn lines can report how many files are out of sync.
type pendingFailures struct {
	mu    sync.Mutex
	files map[string]struct{}
}

func newPendingFailures() *pendingFailures { return &pendingFailures{files: map[string]struct{}{}} }

func (p *pendingFailures) add(rel string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	p.files[rel] = struct{}{}
}

func (p *pendingFailures) remove(rel string) {
	p.mu.Lock()
	defer p.mu.Unlock()
	delete(p.files, rel)
}

func (p *pendingFailures) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.files)
}

// syncRetryBackoff is the wait before each sync retry; tests shrink it.
var syncRetryBackoff = []time.Duration{time.Second, 2 * time.Second}

// transientSyncError reports whether a sync failure is retried: 5xx, 429, a
// non-JSON 404, or a transport error.
func transientSyncError(err error) bool {
	var he *client.HTTPError
	if !errors.As(err, &he) {
		return true
	}
	return he.StatusCode >= 500 || he.StatusCode == http.StatusTooManyRequests || isGatewayNotFound(he)
}

// sendSync sends a doc request, retrying transient failures until ctx is canceled.
func sendSync(ctx context.Context, c *client.Client, req client.RawRequest) error {
	for attempt := 0; ; attempt++ {
		_, err := c.DoRaw(ctx, req)
		if err == nil || ctx.Err() != nil || attempt >= len(syncRetryBackoff) || !transientSyncError(err) {
			return err
		}
		select {
		case <-ctx.Done():
			return ctx.Err()
		case <-time.After(syncRetryBackoff[attempt]):
		}
	}
}

// docReq builds a /doc request for themeID. verb: create (POST stub), patch
// (PATCH content), delete (DELETE by type+location).
func docCreateReq(themeID, typ, loc string) client.RawRequest {
	return client.RawRequest{Method: "POST", Path: themeBaseV202601 + "/" + themeID + "/doc",
		Data: map[string]any{"doc": map[string]any{"type": typ, "location": loc}}}
}
func docPatchReq(themeID, typ, loc, content string) client.RawRequest {
	return client.RawRequest{Method: "PATCH", Path: themeBaseV202601 + "/" + themeID + "/doc",
		Data: map[string]any{"doc": map[string]any{"type": typ, "location": loc, "content": content}}}
}
func docDeleteReq(themeID, typ, loc string) client.RawRequest {
	return client.RawRequest{Method: "DELETE", Path: themeBaseV202601 + "/" + themeID + "/doc",
		Params: map[string]any{"type": typ, "location": loc}}
}

// handleSync routes one fsnotify event into the right /doc call, updates the
// snapshot, broadcasts a livereload refresh, and tracks per-file failure state.
// create/update are an UPSERT keyed on the doctree snapshot (not the event kind,
// since atomic-save renames report as "create" for existing docs).
func handleSync(
	ctx context.Context,
	c *client.Client,
	themeID, kind, rel string,
	snap *doc.FileSnapshot,
	dedup *doc.Deduper,
	pending *pendingFailures,
	lr *watch.LiveReloadServer,
	stderr io.Writer,
) {
	typ, loc, err := doc.ParseThemeFile(rel)
	if err != nil {
		return
	}

	switch kind {
	case "create", "update":
		fi, serr := os.Stat(rel)
		if serr != nil {
			_, _ = fmt.Fprintf(stderr, "[skip] %s: stat failed (%v) — not synced\n", rel, serr)
			return
		}
		if fi.IsDir() {
			return
		}
		content, rerr := os.ReadFile(rel)
		if rerr != nil {
			_, _ = fmt.Fprintf(stderr, "[skip] %s: read failed (%v) — change not synced\n", rel, rerr)
			return
		}
		if !utf8.Valid(content) {
			_, _ = fmt.Fprintf(stderr, "[skip] %s: binary file — run 'themes push' to sync it\n", rel)
			return
		}
		if dedup.Unchanged(rel, content) {
			return
		}
		if !snap.Has(typ, loc) {
			if err := sendSync(ctx, c, docCreateReq(themeID, typ, loc)); err != nil {
				pending.add(rel)
				_, _ = fmt.Fprintf(stderr, "[%s] %s -> FAIL %v   [unsynced: %d]\n", kind, rel, err, pending.count())
				return
			}
			snap.Add(typ, loc)
		}
		if err := sendSync(ctx, c, docPatchReq(themeID, typ, loc, string(content))); err != nil {
			pending.add(rel)
			_, _ = fmt.Fprintf(stderr, "[%s] %s -> FAIL %v   [unsynced: %d]\n", kind, rel, err, pending.count())
			return
		}
		dedup.Record(rel, content)

	case "delete":
		if err := sendSync(ctx, c, docDeleteReq(themeID, typ, loc)); err != nil {
			pending.add(rel)
			_, _ = fmt.Fprintf(stderr, "[delete] %s -> FAIL %v   [unsynced: %d]\n", rel, err, pending.count())
			return
		}
		snap.Remove(typ, loc)
		dedup.Forget(rel)
	}

	pending.remove(rel)
	_ = lr.Refresh(rel)
	suffix := ""
	if n := pending.count(); n > 0 {
		suffix = fmt.Sprintf("   [unsynced: %d]", n)
	}
	_, _ = fmt.Fprintf(stderr, "[%s] %s -> synced%s\n", kind, rel, suffix)
}

// printDriftSummary counts files present on only one side (set difference; the
// doctree carries no content hashes).
func printDriftSummary(w io.Writer, snap doc.FileSnapshot, localRel map[string]struct{}) {
	remote := map[string]struct{}{}
	for typ, locs := range snap {
		for _, loc := range locs {
			remote[typ+"/"+loc] = struct{}{}
		}
	}
	remoteOnly := 0
	for rel := range remote {
		if _, ok := localRel[rel]; !ok {
			remoteOnly++
		}
	}
	localOnly := 0
	for rel := range localRel {
		if _, ok := remote[rel]; !ok {
			localOnly++
		}
	}
	_, _ = fmt.Fprintf(w, "[serve] file list: %d file(s) only on the remote, %d only local\n", remoteOnly, localOnly)
}
