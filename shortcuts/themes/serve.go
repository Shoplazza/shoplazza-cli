package themes

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"sync"
	"syscall"
	"unicode/utf8"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/devstate"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/doc"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/watch"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"

	"github.com/spf13/cobra"
)

// serveShortcut is the `themes serve` workflow: the largest pipeline in
// the themes module. It:
//
//  0. Resolves the target theme. With --theme-id, that theme is served
//     (and its remote files overwritten — v1-parity behavior). Without it,
//     serve runs against a per-directory development theme: the id saved
//     in .shoplazza/theme-state.json (keyed by store host) is reused when
//     the theme still exists; otherwise a fresh dev theme is created by
//     uploading the cwd zip with an EMPTY theme_id (the same
//     /themes/upload contract `themes share` uses) and the new id is
//     saved back. Existing merchant themes are never touched in dev mode.
//  1. Runs an initial push (zip → multipart upload → task polling) so the
//     remote theme matches the cwd starting state. Skipped when dev-theme
//     creation just uploaded the identical tree.
//  2. Pulls the doctree so subsequent incremental syncs know which files
//     already exist on the server (decides POST-create-then-PATCH vs
//     plain-PATCH).
//  3. Binds a LiveReload v6 WebSocket server (default port 21647) and
//     serves /livereload.js from the same port.
//  4. Watches the cwd theme tree via fsnotify; OnCreate / OnUpdate /
//     OnDelete each push the change to the matching v2 spec /doc endpoint
//     and broadcast a livereload refresh to all connected browsers.
//  5. Tracks per-file HTTP sync failures in a pendingFailures set: failures
//     log a warn line with the unsynced count and stay watching; a
//     successful sync of the same file removes it. Fatal watcher errors
//     (EMFILE etc.) and livereload bind failures hard-exit.
//
// serve NEVER modifies any theme file: it only exposes a livereload WebSocket
// and serves the client JS. The initial push runs through pushShortcut.Execute
// directly so its stderr logs, task-polling, and error classification stay
// byte-identical to a standalone `themes push`.
var serveShortcut = common.Shortcut{
	Service: "themes",
	Command: "serve",
	Use:     "serve [--theme-id <id>]",
	Short:   "Upload to a development theme (or --theme-id), watch the current theme, and live-reload browsers",
	Long: `Start a local theme development loop: upload the current directory's theme
files to a remote theme, then watch the directory and push every change,
live-reloading connected browsers.

Run it from a theme directory (one containing config/settings_schema.json).

Two modes:

  Development theme (default, --theme-id omitted):
    The first run creates a dedicated development theme on the store
    ("Development - <theme name>") and writes its id to
    .shoplazza/theme-state.json in the theme directory (one entry per
    store); later runs reuse it. Your store's existing themes are never
    touched. When the work is ready, apply it to a real theme with
    'shoplazza themes push --theme-id <id>'.

  Explicit theme (--theme-id <id>):
    serve uploads to and continuously overwrites that theme's remote files
    with your local copies, starting with a full upload at startup. Only
    point it at a theme whose remote content you intend to replace.

Syncing is one-way (local -> remote). Changes made in the online Theme
Editor are not written back to local files; fetch them with
'shoplazza themes pull'.`,
	// Long-running watch process, so blind scans skip it.
	NotScannable: true,
	Flags: []common.Flag{
		{
			Name:  "theme-id",
			Short: "t",
			Type:  common.FlagString,
			Description: "Theme ID to serve and overwrite (optional). Omit to use a per-directory development theme. " +
				"Run `shoplazza themes list` to discover IDs.",
		},
		{
			Name:        "port",
			Type:        common.FlagInt,
			Default:     21647,
			Description: "LiveReload server port",
		},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		explicitID := in.Flags.GetString("theme-id")
		if err := theme.ValidateThemeID(explicitID); err != nil {
			return common.ExecResult{}, err
		}
		port := in.Flags.GetInt("port")
		// Validate the port range up front: an out-of-range value previously
		// surfaced as a network-class bind failure with a misleading
		// "another instance may be running" hint. 0 stays allowed — it is
		// the documented ephemeral-port seam (tests, parallel runs).
		if port < 0 || port > 65535 {
			return common.ExecResult{}, theme.ErrValidation(
				"invalid --port %d: must be between 1 and 65535", port)
		}
		prog := output.NewProgress(os.Stderr)

		clientBase := ""
		if in.Client != nil {
			clientBase = in.Client.BaseURL
		}
		storeKey := devstate.StoreKey(clientBase)

		// Dry-run. Explicit mode previews the four live plans (detail +
		// upload + task-poll + doctree). Dev mode reads the state file (no
		// writes): a saved id previews the reuse path; no saved id previews
		// the create path (upload with an empty theme_id, its task poll, and
		// a doctree fetch with a placeholder id).
		if in.DryRun {
			if explicitID != "" {
				return common.ExecResult{Plans: []common.PlannedRequest{
					PlanDetail(explicitID),
					PlanUpload(explicitID, "<theme_name>", "<theme_version>"),
					PlanTaskDetail("<task_id-from-upload>"),
					PlanDocTree(explicitID),
				}}, nil
			}
			cwd, gerr := os.Getwd()
			if gerr != nil {
				return common.ExecResult{}, theme.ErrLocalIO("getwd", gerr)
			}
			if savedID, ok := devstate.Load(cwd, storeKey); ok {
				return common.ExecResult{Plans: []common.PlannedRequest{
					PlanDetail(savedID),
					PlanUpload(savedID, "<theme_name>", "<theme_version>"),
					PlanTaskDetail("<task_id-from-upload>"),
					PlanDocTree(savedID),
				}}, nil
			}
			name, version, _ := readThemeInfo(cwd)
			if name == "" {
				name = "<theme>"
			}
			if version == "" {
				version = "<version>"
			}
			return common.ExecResult{Plans: []common.PlannedRequest{
				PlanShareUpload("", devThemeName(name), version),
				PlanTaskDetail("<task_id-from-upload>"),
				PlanDocTree("<dev_theme_id>"),
			}}, nil
		}

		cwd, err := os.Getwd()
		if err != nil {
			return common.ExecResult{}, theme.ErrLocalIO("getwd", err)
		}

		// Resolve the target theme. Explicit --theme-id serves (and
		// overwrites) that theme. Otherwise serve runs against a
		// per-directory development theme: reuse the id saved in
		// .shoplazza/theme-state.json when the theme still exists remotely;
		// create a fresh one (and save its id) when there is no record or
		// the saved theme 404s (deleted remotely).
		themeID := explicitID
		initialPushDone := false
		if themeID == "" {
			if savedID, ok := devstate.Load(cwd, storeKey); ok {
				if _, derr := common.Send(ctx, in.Client, PlanDetail(savedID)); derr == nil {
					themeID = savedID
					prog.Begin(fmt.Sprintf("[serve] development theme: %s (reused from %s)",
						savedID, filepath.ToSlash(filepath.Join(".shoplazza", "theme-state.json")))).Done()
				} else if !isHTTPNotFound(derr) {
					return common.ExecResult{}, classifyHTTPErr(derr, savedID)
				}
				// 404 → stale record; fall through and recreate.
			}
			if themeID == "" {
				name, version, rerr := readThemeInfo(cwd)
				if rerr != nil {
					return common.ExecResult{}, rerr
				}
				step := prog.Begin("[serve] creating development theme")
				newID, cerr := createDevTheme(ctx, in.Client, cwd, devThemeName(name), version)
				if cerr != nil {
					step.Fail()
					return common.ExecResult{}, cerr
				}
				step.Done()
				if serr := devstate.Save(cwd, storeKey, newID); serr != nil {
					return common.ExecResult{}, theme.ErrLocalIO("write .shoplazza/theme-state.json", serr)
				}
				fmt.Fprintf(os.Stderr, "[serve] development theme %s created (id saved to %s)\n",
					newID, filepath.ToSlash(filepath.Join(".shoplazza", "theme-state.json")))
				themeID = newID
				// The create upload already pushed the cwd tree — the
				// initial push would re-upload identical bytes.
				initialPushDone = true
			}
		} else {
			prog.Begin(fmt.Sprintf("[serve] target theme: %s", themeID)).Done()
		}

		// Step 1: initial push. Delegate to pushShortcut.Execute (it owns the
		// detail → pack → upload → poll pipeline), passing a fresh ExecInput
		// with just the --theme-id flag. Skipped when the dev-theme creation
		// just uploaded the same tree.
		if !initialPushDone {
			pushIn := common.ExecInput{
				Client: in.Client,
				Flags:  buildPushFlagSet(themeID),
			}
			if _, perr := pushShortcut.Execute(ctx, pushIn); perr != nil {
				return common.ExecResult{}, perr
			}
		}

		// Step 2: pull the doctree snapshot. This tells handleSync whether
		// a given (type, location) already exists on the server, so an
		// OnUpdate event can decide between PATCH (in-place edit) and
		// POST-then-PATCH (create then fill).
		dtStep := prog.Begin("[serve] syncing doctree")
		dtResp, err := common.Send(ctx, in.Client, PlanDocTree(themeID))
		if err != nil {
			dtStep.Fail()
			return common.ExecResult{}, classifyHTTPErr(err, themeID)
		}
		snap := doc.FromDocTreeResponse(dtResp)
		dtStep.Done()

		// Step 3: bind the LiveReload server. Port 0 → ephemeral (tests).
		// Bind failure is fatal — there's no useful degraded mode (the
		// whole point of serve is the browser-refresh bridge).
		lr := watch.NewLiveReloadServer(port)
		lrCtx, lrCancel := context.WithCancel(ctx)
		defer lrCancel()
		if err := lr.Start(lrCtx); err != nil {
			return common.ExecResult{}, theme.ErrLiveReloadBindFailed(port, err)
		}
		defer lr.Close()
		prog.Begin(fmt.Sprintf("[serve] livereload server: ws://localhost:%d", lr.Port())).Done()

		// Step 4: start the file watcher. Recursive over the 8 standard
		// theme dirs (assets/blocks/config/layout/locales/sections/
		// snippets/templates). OnError → watcher fatal channel for hard
		// exit; everything else dispatches to handleSync.
		pending := newPendingFailures()
		watchErrCh := make(chan error, 1)
		// Compile the .themeignore matcher ONCE at serve start so the watch
		// filter honors the same exclusion rules `themes push` applies —
		// previously an ignored file edited during serve was still pushed.
		ignorer, ierr := pack.LoadThemeIgnorer(cwd, "")
		if ierr != nil {
			return common.ExecResult{}, theme.ErrLocalIO("load .themeignore", ierr)
		}
		watchFilter := buildWatchFilter(ignorer)

		// Content-dedup: the initial push already uploaded the cwd tree, so seed
		// each file's hash as "already synced" so metadata-only events with
		// unchanged content are skipped. The walk covers only the 8 standard
		// theme dirs (mirroring watch.Watch / pack.EnumerateThemeFiles).
		dedup := doc.NewDeduper()
		for _, d := range pack.ThemeDirs {
			base := filepath.Join(cwd, d)
			if _, serr := os.Stat(base); serr != nil {
				continue
			}
			_ = filepath.Walk(base, func(p string, info os.FileInfo, err error) error {
				if err != nil {
					return nil
				}
				if info.IsDir() {
					if b := filepath.Base(p); p != base && len(b) > 0 && b[0] == '.' {
						return filepath.SkipDir // hidden dirs are never synced
					}
					return nil
				}
				rel, rerr := filepath.Rel(cwd, p)
				if rerr != nil {
					return nil
				}
				relSlash := filepath.ToSlash(rel)
				if !watchFilter(relSlash) {
					return nil
				}
				if b, e := os.ReadFile(p); e == nil {
					dedup.Record(relSlash, b)
				}
				return nil
			})
		}

		stop, err := watch.Watch(cwd, watch.WatchOptions{Filter: watchFilter}, watch.Callback{
			OnCreate: func(rel string) {
				handleSync(ctx, in.Client, themeID, "create", rel, &snap, dedup, pending, lr, os.Stderr)
			},
			OnUpdate: func(rel string) {
				handleSync(ctx, in.Client, themeID, "update", rel, &snap, dedup, pending, lr, os.Stderr)
			},
			OnDelete: func(rel string) {
				handleSync(ctx, in.Client, themeID, "delete", rel, &snap, dedup, pending, lr, os.Stderr)
			},
			OnError: func(werr error) {
				// Non-blocking: if the channel is full, drop — one fatal
				// is enough to drive the shutdown select below.
				select {
				case watchErrCh <- werr:
				default:
				}
			},
		})
		if err != nil {
			return common.ExecResult{}, theme.ErrWatcherFatal(err)
		}
		defer stop()

		// Step 5: print the v1-verbatim two-URL banner. Best-effort shop
		// lookup — if the shop endpoint fails we still print the banner
		// with a placeholder domain rather than hard-failing.
		storeDomain := extractStoreDomainBest(ctx, in.Client)
		printV1ServeBanner(os.Stderr, storeDomain, themeID)
		fmt.Fprintln(os.Stderr, "Listening for file changes ...")

		// Step 6: block until ctx cancel, SIGINT/SIGTERM, or fatal watcher
		// error. fsnotify EMFILE / livereload runtime failure → hard exit
		// (no auto-restart). All defers above unwind the watcher and LR
		// server in reverse order.
		sigCh := make(chan os.Signal, 1)
		signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
		defer signal.Stop(sigCh)

		select {
		case <-ctx.Done():
		case <-sigCh:
		case werr := <-watchErrCh:
			return common.ExecResult{}, theme.ErrWatcherFatal(werr)
		}
		return common.ExecResult{Body: map[string]any{"status": "stopped"}}, nil
	},
}

// buildWatchFilter returns serve's file filter: keep only real theme-tree
// files, skipping editor temp/swap/hidden artifacts and any path matched by
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

// buildPushFlagSet creates a FlagSet exposing only --theme-id (the one flag
// pushShortcut consults). A throwaway cobra.Command gives the FlagSet the same
// runtime type pushShortcut sees on the normal CLI path, so the initial push
// runs byte-for-byte like a standalone `themes push -t <id>`.
func buildPushFlagSet(themeID string) common.FlagSet {
	cmd := &cobra.Command{Use: "push"}
	cmd.Flags().StringP("theme-id", "t", themeID, "")
	return common.NewCobraFlagSet(cmd)
}

// pendingFailures is a thread-safe set of relative file paths that failed
// their last sync attempt. handleSync writes warn lines that include the
// current set size so operators can see at a glance how many files are
// out of sync. Following the stderr one-line-per-event convention, the set
// itself is not surfaced through any other channel.
type pendingFailures struct {
	mu    sync.Mutex
	files map[string]struct{}
}

func newPendingFailures() *pendingFailures {
	return &pendingFailures{files: map[string]struct{}{}}
}

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

// count returns the current pending-failure count under one lock acquire.
// Avoids the list-then-len pattern in the hot path of every successful sync.
func (p *pendingFailures) count() int {
	p.mu.Lock()
	defer p.mu.Unlock()
	return len(p.files)
}

// handleSync routes a single fsnotify event into the right v2 spec /doc
// API call, updates the in-memory snapshot, broadcasts a livereload
// refresh, and tracks per-file failure state. Called from the watcher's
// internal goroutine — must be quick (per watch.Callback docs).
//
// Event semantics — create/update are an UPSERT keyed on the doctree snapshot,
// not the fsnotify event kind (atomic-save renames look like "create" even for
// existing docs; deciding by snapshot mirrors v1's existInList):
//
//	create/update → snapshot.Has ? PATCH content
//	                             : POST {type,location} stub + snapshot.Add, then PATCH content
//	delete        → DELETE /themes/{id}/doc?type=&location=; snapshot.Remove
//
// HTTP failure → file added to pendingFailures, warn line emitted, NO
// livereload broadcast (the browser still has a stale-but-known state;
// refreshing without a successful sync would flicker the merchant view).
// Success → file removed from pendingFailures, lr.Refresh fires.
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
	// ParseThemeFile rejects anything outside the 8 theme dirs (including
	// .themeignore, README.md, etc.). The watcher Filter already does
	// this — handleSync rechecks defensively.
	typ, loc, err := doc.ParseThemeFile(rel)
	if err != nil {
		return
	}

	switch kind {
	case "create", "update":
		// Guard rails before any wire traffic:
		//   - vanished file → skip; a Remove event will follow.
		//   - directory → skip; syncing one would create a phantom remote doc.
		//   - read failure → skip with a note; pushing empty content from a
		//     failed read previously truncated the remote file.
		//   - invalid UTF-8 (binary asset) → skip with a note; the JSON doc
		//     patch would mangle every non-UTF-8 byte to U+FFFD on the wire.
		fi, serr := os.Stat(rel)
		if serr != nil {
			fmt.Fprintf(stderr, "[skip] %s: stat failed (%v) — not synced\n", rel, serr)
			return
		}
		if fi.IsDir() {
			return
		}
		content, rerr := os.ReadFile(rel)
		if rerr != nil {
			fmt.Fprintf(stderr, "[skip] %s: read failed (%v) — change not synced\n", rel, rerr)
			return
		}
		if !utf8.Valid(content) {
			fmt.Fprintf(stderr, "[skip] %s: binary file — run 'themes push' to sync it\n", rel)
			return
		}
		if dedup.Unchanged(rel, content) {
			return // identical content → nothing to push, no refresh, no log
		}
		// Upsert keyed on server-existence (the doctree snapshot), not the
		// fsnotify event kind: atomic-save renames report as "create" even for
		// existing docs, and POSTing a create-stub for an existing doc 500s.
		if !snap.Has(typ, loc) {
			// New doc: the server requires a stub to exist before any /doc PATCH.
			if _, err := common.Send(ctx, c,
				PlanDocCreate(themeID, map[string]any{"type": typ, "location": loc})); err != nil {
				pending.add(rel)
				fmt.Fprintf(stderr, "[%s] %s -> FAIL %v   [unsynced: %d]\n",
					kind, rel, err, pending.count())
				return
			}
			snap.Add(typ, loc)
		}
		if _, err := common.Send(ctx, c, PlanDocPatch(themeID,
			map[string]any{"type": typ, "location": loc, "content": string(content)})); err != nil {
			pending.add(rel)
			fmt.Fprintf(stderr, "[%s] %s -> FAIL %v   [unsynced: %d]\n",
				kind, rel, err, pending.count())
			return
		}
		dedup.Record(rel, content) // record only after a successful push so failures retry

	case "delete":
		// Query-string parameters per spec — body is empty on DELETE.
		if _, err := common.Send(ctx, c, PlanDocDelete(themeID,
			map[string]any{"type": typ, "location": loc})); err != nil {
			pending.add(rel)
			fmt.Fprintf(stderr, "[delete] %s -> FAIL %v   [unsynced: %d]\n",
				rel, err, pending.count())
			return
		}
		snap.Remove(typ, loc)
		dedup.Forget(rel)
	}

	// All-success path: clear any prior failure flag, broadcast the
	// refresh, log the synced line with the (possibly empty) unsynced
	// suffix so operators can confirm the queue is draining.
	pending.remove(rel)
	_ = lr.Refresh(rel)
	suffix := ""
	if n := pending.count(); n > 0 {
		suffix = fmt.Sprintf("   [unsynced: %d]", n)
	}
	fmt.Fprintf(stderr, "[%s] %s -> synced%s\n", kind, rel, suffix)
}

// extractStoreDomainBest fetches the shop's primary domain for the banner.
// Best-effort: errors degrade to "<unknown-shop>" so the banner still
// prints (the URLs are degraded but the rest of the workflow is unaffected).
func extractStoreDomainBest(ctx context.Context, c *client.Client) string {
	resp, err := common.Send(ctx, c, PlanShop())
	if err != nil {
		return "<unknown-shop>"
	}
	d := extractStoreDomain(resp)
	if d == "" {
		return "<unknown-shop>"
	}
	return d
}

// printV1ServeBanner emits the two-URL banner verbatim from the v1 CLI. The
// blank lines are intentional, separating the preview URL from the editor URL.
func printV1ServeBanner(w io.Writer, domain, themeID string) {
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Please open this URL in your browser:")
	fmt.Fprintf(w, "   https://%s/?preview_theme_id=%s\n", domain, themeID)
	fmt.Fprintln(w, "")
	fmt.Fprintln(w, "Customize this theme in the Theme Editor, and use 'themes pull' to get the changes:")
	fmt.Fprintf(w, "   https://%s/admin/smart_apps/editor?theme_id=%s\n", domain, themeID)
	fmt.Fprintln(w, "")
}
