package themecmd

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"path/filepath"
	"syscall"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/devstate"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/doc"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/pack"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/watch"
)

const shopV202601 = "/openapi/2026-01/shop"

// newCmdServe builds `themes serve`: upload to a development theme (or an
// explicit --theme-id), watch the theme tree, push each change to the matching
// /doc endpoint, and live-reload connected browsers. Owns its -e-aware store
// client (like push/pull); long-running, so it is NotScannable.
func newCmdServe(f *cmdutil.Factory) *cobra.Command {
	var themeIDFlag, taskID, environment string
	var skipPush bool
	var port int
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Upload to a development theme (or --theme-id), watch the current theme, and live-reload browsers",
		Long: `Start a local theme development loop: upload the current directory's theme
files to a remote theme, then watch the directory and push every change,
live-reloading connected browsers. Run it from a theme directory (one containing
config/settings_schema.json).

Development theme (default): the first run creates "Development - <name>" and
records its id in .shoplazza/theme-state.json (one per store); later runs reuse
it. Existing themes are never touched. Explicit --theme-id (or -e's theme):
serve uploads to and continuously overwrites that theme. Syncing is one-way
(local -> remote); fetch editor changes with 'shoplazza themes pull'.`,
		Example: `  # Serve against a per-directory development theme
  shoplazza themes serve

  # Serve and overwrite a specific theme
  shoplazza themes serve --theme-id 123456`,
		Annotations: map[string]string{
			cmdutil.AnnotationAuthFree:     "true", // owns its (env-aware) auth
			cmdutil.AnnotationNotScannable: "true", // long-running watch process
		},
		Args: cobra.NoArgs,
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			if port < 1 || port > 65535 {
				return theme.ErrValidation("invalid --port %d: must be between 1 and 65535", port)
			}
			rs, err := resolveStore(ctx, f, cmd)
			if err != nil {
				return err
			}
			// -e's theme= seeds explicit mode when --theme-id is omitted.
			explicitID := themeIDFlag
			if explicitID == "" {
				explicitID = rs.Env.Theme
			}
			if err := theme.ValidateThemeID(explicitID); err != nil {
				return err
			}
			if skipPush && taskID != "" {
				return theme.ErrValidation(
					"--skip-push conflicts with --task-id: the task it waits for is itself a full upload; pass only one")
			}

			c := rs.Client
			stderr := cmd.ErrOrStderr()
			prog := output.NewProgress(stderr)
			storeKey := devstate.StoreKey(c.BaseURL)

			cwd, err := os.Getwd()
			if err != nil {
				return theme.ErrLocalIO("getwd", err)
			}

			// Resolve the target theme (explicit / task-resume / dev-theme).
			themeID := explicitID
			initialPushDone := false
			switch {
			case taskID != "":
				var name string
				if themeID != "" {
					if _, derr := c.DoRaw(ctx, detailReq(themeID)); derr != nil {
						return classifyHTTPErr(derr, themeID)
					}
					prog.Begin("[serve] target theme: " + themeID).Done()
				} else {
					if name, _, err = theme.ReadInfo(cwd); err != nil {
						return err
					}
				}
				_, _ = fmt.Fprintf(stderr, "[serve] resuming upload task %s\n", taskID)
				step := prog.Begin("[serve] waiting for the server to process the theme")
				payload, werr := waitUploadTask(ctx, c, taskID)
				if werr != nil {
					step.Fail()
					return werr
				}
				step.Done()
				if themeID == "" {
					if themeID = themeIDFromTask(payload); themeID == "" {
						return theme.ErrValidation("task %s did not report a theme id; re-run with --theme-id <id>", taskID)
					}
					if aerr := adoptDevTheme(ctx, c, prog, cwd, storeKey, themeID, devThemeName(name)); aerr != nil {
						return aerr
					}
				}
				initialPushDone = true

			case themeID == "":
				if savedID, ok := devstate.Load(cwd, storeKey); ok {
					if _, derr := c.DoRaw(ctx, detailReq(savedID)); derr == nil {
						themeID = savedID
						prog.Begin(fmt.Sprintf("[serve] development theme: %s (reused from %s)",
							savedID, filepath.ToSlash(filepath.Join(".shoplazza", "theme-state.json")))).Done()
					} else if !isHTTPNotFound(derr) {
						return classifyHTTPErr(derr, savedID)
					}
				}
				if themeID == "" {
					if skipPush {
						return errSkipPushNoDevTheme()
					}
					name, version, rerr := theme.ReadInfo(cwd)
					if rerr != nil {
						return rerr
					}
					newID, cerr := createDevTheme(ctx, c, prog, cwd, devThemeName(name), version)
					if cerr != nil {
						return cerr
					}
					if aerr := adoptDevTheme(ctx, c, prog, cwd, storeKey, newID, devThemeName(name)); aerr != nil {
						return aerr
					}
					themeID = newID
					initialPushDone = true // the create upload already pushed the cwd tree
				}

			default:
				if skipPush {
					if _, derr := c.DoRaw(ctx, detailReq(themeID)); derr != nil {
						return classifyHTTPErr(derr, themeID)
					}
				}
				prog.Begin("[serve] target theme: " + themeID).Done()
			}

			// Step 1: initial push (unless dev-theme creation already uploaded, or
			// --skip-push). Reuses the shared push core.
			if skipPush {
				prog.Begin("[serve] skipping the startup upload (--skip-push)").Done()
			} else if !initialPushDone {
				if _, perr := pushTheme(ctx, prog, stderr, c, themeID, ""); perr != nil {
					return perr
				}
			}

			// Step 2: doctree snapshot decides PATCH vs POST-then-PATCH per file.
			dtStep := prog.Begin("[serve] syncing doctree")
			dtResp, err := c.DoRaw(ctx, client.RawRequest{Method: "GET", Path: themeBaseV202601 + "/" + themeID + "/doctree"})
			if err != nil {
				dtStep.Fail()
				return classifyHTTPErr(err, themeID)
			}
			snap := doc.FromDocTreeResponse(asMap(dtResp.Body))
			dtStep.Done()

			// Step 3: LiveReload server (bind failure is fatal).
			lr := watch.NewLiveReloadServer(port)
			lrCtx, lrCancel := context.WithCancel(ctx)
			defer lrCancel()
			if err := lr.Start(lrCtx); err != nil {
				return theme.ErrLiveReloadBindFailed(port, err)
			}
			defer func() { _ = lr.Close() }()
			prog.Begin(fmt.Sprintf("[serve] livereload server: ws://localhost:%d", lr.Port())).Done()

			// Step 4: file watcher + dedup seeding.
			pending := newPendingFailures()
			watchErrCh := make(chan error, 1)
			ignorer, ierr := pack.LoadThemeIgnorer(cwd, "")
			if ierr != nil {
				return theme.ErrLocalIO("load .themeignore", ierr)
			}
			watchFilter := buildWatchFilter(ignorer)

			dedup := doc.NewDeduper()
			localRel := seedDedup(cwd, watchFilter, dedup)
			if skipPush {
				printDriftSummary(stderr, snap, localRel)
			}

			stop, werr := watch.Watch(cwd, watch.WatchOptions{Filter: watchFilter}, watch.Callback{
				OnCreate: func(rel string) { handleSync(ctx, c, themeID, "create", rel, &snap, dedup, pending, lr, stderr) },
				OnUpdate: func(rel string) { handleSync(ctx, c, themeID, "update", rel, &snap, dedup, pending, lr, stderr) },
				OnDelete: func(rel string) { handleSync(ctx, c, themeID, "delete", rel, &snap, dedup, pending, lr, stderr) },
				OnError: func(e error) {
					select {
					case watchErrCh <- e:
					default:
					}
				},
			})
			if werr != nil {
				return theme.ErrWatcherFatal(werr)
			}
			defer stop()

			// Step 5: banner (best-effort shop domain).
			printV1ServeBanner(stderr, extractStoreDomainBest(ctx, c), themeID)
			_, _ = fmt.Fprintln(stderr, "Listening for file changes ...")

			// Step 6: block until ctx cancel, SIGINT/SIGTERM, or a fatal watcher error.
			sigCh := make(chan os.Signal, 1)
			signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
			defer signal.Stop(sigCh)
			select {
			case <-ctx.Done():
			case <-sigCh:
			case e := <-watchErrCh:
				return theme.ErrWatcherFatal(e)
			}
			return output.PrintAPISuccess(cmd.OutOrStdout(), map[string]any{"status": "stopped"}, cmdutil.GetFormat(cmd), "")
		},
	}
	cmd.Flags().StringVarP(&themeIDFlag, "theme-id", "t", "", "Theme ID to serve and overwrite (optional; omit for a per-directory development theme)")
	cmd.Flags().StringVar(&taskID, "task-id", "", "Resume waiting for an earlier upload task instead of uploading again")
	cmd.Flags().StringVarP(&environment, "environment", "e", "", "Environment from shoplazza.theme.toml (store/profile/theme); see 'themes env list'")
	cmd.Flags().BoolVar(&skipPush, "skip-push", false, "Skip the startup full upload and watch right away (one-way sync of changed files only)")
	cmd.Flags().IntVar(&port, "port", 21647, "LiveReload server port")
	return cmd
}

// detailReq is the theme-detail existence check (GET /themes/{id}).
func detailReq(themeID string) client.RawRequest {
	return client.RawRequest{Method: "GET", Path: themeBaseV202601 + "/" + themeID}
}

// seedDedup walks the 8 standard theme dirs, seeds each kept file's hash as
// "already synced" (the initial push uploaded the tree), and returns the local
// relative-path set for the --skip-push drift summary.
func seedDedup(cwd string, keep func(string) bool, dedup *doc.Deduper) map[string]struct{} {
	localRel := map[string]struct{}{}
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
					return filepath.SkipDir
				}
				return nil
			}
			rel, rerr := filepath.Rel(cwd, p)
			if rerr != nil {
				return nil
			}
			relSlash := filepath.ToSlash(rel)
			if !keep(relSlash) {
				return nil
			}
			localRel[relSlash] = struct{}{}
			if b, e := os.ReadFile(p); e == nil {
				dedup.Record(relSlash, b)
			}
			return nil
		})
	}
	return localRel
}

// errSkipPushNoDevTheme rejects --skip-push when there is no development theme
// (creating one is itself a full upload).
func errSkipPushNoDevTheme() error {
	return theme.ErrValidation(
		"--skip-push needs an existing development theme on this store; creating one requires " +
			"a full upload. Re-run without --skip-push, or pass --theme-id <id>.")
}

// extractStoreDomainBest fetches the shop's primary domain for the banner;
// errors degrade to "<unknown-shop>" so the banner still prints.
func extractStoreDomainBest(ctx context.Context, c *client.Client) string {
	resp, err := c.DoRaw(ctx, client.RawRequest{Method: "GET", Path: shopV202601})
	if err != nil {
		return "<unknown-shop>"
	}
	if d := extractStoreDomain(asMap(resp.Body)); d != "" {
		return d
	}
	return "<unknown-shop>"
}

// extractStoreDomain digs the shop domain out of the response envelope shapes
// (root, root.shop, root.data, root.data.shop), preferring "domain" then the v1
// "store_domain" alias. Shared by serve (v2 /shop) and share (v1 /shop).
func extractStoreDomain(resp map[string]any) string {
	for _, m := range []map[string]any{resp, mapChild(resp, "shop"), mapChild(resp, "data"), mapChild(mapChild(resp, "data"), "shop")} {
		if m == nil {
			continue
		}
		if d, ok := m["domain"].(string); ok && d != "" {
			return d
		}
		if d, ok := m["store_domain"].(string); ok && d != "" {
			return d
		}
	}
	return ""
}

// printV1ServeBanner emits the two-URL banner verbatim from the v1 CLI.
func printV1ServeBanner(w io.Writer, domain, themeID string) {
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Please open this URL in your browser:")
	_, _ = fmt.Fprintf(w, "   https://%s/?preview_theme_id=%s\n", domain, themeID)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Customize this theme in the Theme Editor, and use 'themes pull' to get the changes:")
	_, _ = fmt.Fprintf(w, "   https://%s/admin/smart_apps/editor?theme_id=%s\n", domain, themeID)
	_, _ = fmt.Fprintln(w, "")
}
