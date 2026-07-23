package theme_extension

import (
	"context"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/internal/ossupload"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/doc"
	"github.com/Shoplazza/shoplazza-cli/internal/theme/watch"
	te "github.com/Shoplazza/shoplazza-cli/internal/theme_extension"
)

func newCmdServe(f *cmdutil.Factory) *cobra.Command {
	var projectRoot, themeID string
	cmd := &cobra.Command{
		Use:   "serve",
		Short: "Register + push a dev build, then sync each saved file incrementally (create/update/delete via dev-doc)",
		// Long-running watch process.
		Annotations: map[string]string{cmdutil.AnnotationNotScannable: "true"},
		PreRunE: func(cmd *cobra.Command, _ []string) error {
			if themeID == "" {
				return output.ErrValidation("--theme-id/-t is required (run `shop themes list` to find a theme id)")
			}
			return requireLogin(cmd.Context(), f)
		},
		RunE: func(cmd *cobra.Command, _ []string) error {
			ctx := cmd.Context()
			root := projectRoot
			cfg, err := te.ReadConfig(root)
			if err != nil {
				if errors.Is(err, fs.ErrNotExist) {
					return output.ErrValidation("not a te project (missing extension.config.json in %s)", root)
				}
				// Present but undecodable: do NOT suggest re-registering — the
				// corrupt file still holds the extension_id.
				return output.ErrValidation("%v", err)
			}
			// serve always targets the current store (no --store-domain flag);
			// storeClient("") falls back to the current profile's store.
			store, domain, cErr := storeClient(ctx, f, "")
			if cErr != nil {
				return cErr
			}

			// Progress for the slow push steps goes to stderr while serve's banner
			// and per-file "synced:" lines stay on stdout.
			prog := output.NewProgress(cmd.ErrOrStderr())

			// Build+upload+(register if first time)+initial dev-doctree push.
			pushFull := func() *output.ExitError {
				zip := prog.Begin("Zipping theme-app")
				zipPath, zErr := te.ZipThemeApp(root)
				if zErr != nil {
					zip.Fail()
					if errors.Is(zErr, te.ErrThemeAppMissing) {
						return output.ErrValidation("zip theme-app/: %v (is there a theme-app/ directory?)", zErr)
					}
					return output.ErrInternal("zip theme-app/: %v", zErr)
				}
				zip.Done()

				upStep := prog.Begin("Uploading theme-app")
				up := &ossupload.Uploader{Client: store, HTTPClient: &http.Client{Timeout: 60 * time.Second}}
				resourceURL, uErr := up.Upload(ctx, zipPath)
				if uErr != nil {
					upStep.Fail()
					return uErr
				}
				upStep.Done()
				// Uploaded — best-effort removal keeps .te-build/ from growing one
				// zip per serve. Kept on failure above for debugging.
				_ = os.Remove(zipPath)

				if cfg.ExtensionID == "" {
					reg := prog.Begin("Registering extension")
					res, rErr := te.Register(ctx, store, root, cfg.Name, resourceURL, "", "", time.Second, 10)
					if rErr != nil {
						reg.Fail()
						return rErr
					}
					reg.Done()
					// Use the id Register returns (already persisted to the toml); re-reading
					// risks a zero-value cfg, leaving PushDevDoctree with an empty id.
					cfg.ExtensionID = res.ExtensionID
				}

				push := prog.Begin("Pushing dev doctree")
				if ex := te.PushDevDoctree(ctx, store, cfg.ExtensionID, resourceURL, time.Second, 120); ex != nil {
					push.Fail()
					return ex
				}
				push.Done()
				return nil
			}
			if ex := pushFull(); ex != nil {
				return ex
			}
			// Watch theme-app/ and sync each change incrementally via the per-file
			// dev-doc endpoints: create→POST, update→PATCH, delete→DELETE (v1 parity:
			// lib/theme-extension startWatcher). Per-file errors go to stderr and the
			// dev loop continues — a transient push failure shouldn't kill the
			// watcher. assets-manifest.json is build output → ignored, as v1.
			themeApp := filepath.Join(root, "theme-app")

			// Print v1's two preview URLs (Admin + Storefront, keyed on theme_id +
			// ext_debug; --theme-id is required by PreRunE).
			printServeBanner(cmd.OutOrStdout(), domain, cfg.ExtensionID, themeID)

			filter := newServeFilter(cmd.ErrOrStderr())

			// pushFull just uploaded the whole tree, so seed every file's hash as
			// "already synced". Then metadata-only events whose content is unchanged
			// are skipped instead of re-pushing identical bytes.
			dedup := doc.NewDeduper()
			_ = filepath.Walk(themeApp, func(p string, info os.FileInfo, err error) error {
				if err != nil || info.IsDir() {
					return nil
				}
				rel, rerr := filepath.Rel(themeApp, p)
				if rerr != nil {
					return nil
				}
				relSlash := filepath.ToSlash(rel)
				if !filter(relSlash) {
					return nil
				}
				if b, e := os.ReadFile(p); e == nil {
					dedup.Record(relSlash, b)
				}
				return nil
			})

			onFile := func(op string) func(string) {
				return func(rel string) {
					syncDevDocFile(ctx, store, cfg.ExtensionID, themeApp, op, rel, dedup,
						cmd.OutOrStdout(), cmd.ErrOrStderr())
				}
			}
			// Fatal watcher errors (EMFILE, watcher loss, permission revoked) must
			// not be silently ignored — surface them and stop, instead of blocking
			// forever on a dead watcher. Mirrors themes serve's OnError handling.
			watchErrCh := make(chan error, 1)
			stop, wErr := watch.Watch(themeApp, watch.WatchOptions{Filter: filter}, watch.Callback{
				OnCreate: onFile("create"),
				OnUpdate: onFile("update"),
				OnDelete: onFile("delete"),
				OnError: func(werr error) {
					select {
					case watchErrCh <- werr:
					default:
					}
				},
			})
			if wErr != nil {
				return output.ErrInternal("start watcher: %v", wErr)
			}
			defer stop()
			fmt.Fprintln(cmd.OutOrStdout(), "Listening for file changes ...")
			select {
			case <-ctx.Done():
			case werr := <-watchErrCh:
				return output.ErrInternal("watcher stopped: %v", werr)
			}
			return nil
		},
	}
	cmd.Flags().StringVarP(&themeID, "theme-id", "t", "", "Theme to preview the extension on (required); prints the v1-style Admin + Storefront preview URLs")
	cmd.Flags().StringVar(&projectRoot, "path", ".", "te project root")
	return cmd
}

// newServeFilter keeps only files the per-file dev-doc API can address: valid
// theme files directly under one type dir (<dir>/<file>); editor temp/swap and
// assets-manifest.json (build output) are dropped. Nested paths are rejected
// because DevDocTarget derives (type, location) from the immediate parent dir,
// so assets/sub/img.png would be sent as type "sub" the API doesn't know; a
// one-time stderr note explains the first skip. The returned func may run on the
// watcher goroutine and the seeding walk, so the note is guarded by a sync.Once.
func newServeFilter(stderr io.Writer) func(string) bool {
	var nestedNote sync.Once
	return func(rel string) bool {
		_, loc, err := doc.ParseThemeFile(rel)
		if err != nil {
			return false
		}
		if strings.Contains(loc, "/") {
			nestedNote.Do(func() {
				fmt.Fprintf(stderr, "[skip] %s: nested paths are not supported by the dev-doc API — keep files directly under the type directory\n", rel)
			})
			return false
		}
		if doc.IsEditorTemp(rel) { // skip editor temp/swap/backup (e.g. *.sb-XXXX atomic-save), as themes serve does
			return false
		}
		return path.Base(rel) != "assets-manifest.json"
	}
}

// syncDevDocFile pushes one watched change through the per-file dev-doc API.
// Errors are reported to stderr and swallowed — the dev loop must survive a
// transient per-file failure.
func syncDevDocFile(ctx context.Context, store *client.Client, extensionID, themeApp, op, rel string, dedup *doc.Deduper, stdout, stderr io.Writer) {
	fileType, location := te.DevDocTarget(rel)
	var ex *output.ExitError
	switch op {
	case "create", "update":
		content, rErr := os.ReadFile(filepath.Join(themeApp, filepath.FromSlash(rel)))
		if rErr != nil {
			fmt.Fprintf(stderr, "read %s: %v\n", rel, rErr)
			return
		}
		// Invalid UTF-8 (binary asset) must NEVER ride the JSON dev-doc body —
		// every non-UTF-8 byte would be mangled to U+FFFD on the wire.
		if !utf8.Valid(content) {
			fmt.Fprintf(stderr, "[skip] %s: binary file — restart 'te serve' (or run 'te build') to push it via the bundle upload\n", rel)
			return
		}
		// Skip when content is identical to the last synced version — metadata-only
		// events fire with no real change, so re-pushing identical bytes is noise.
		if dedup.Unchanged(rel, content) {
			return
		}
		// Upsert: PATCH, and create-on-missing. The watcher can't reliably tell
		// create from update for atomic-save editors, and the bulk dev-doctree push
		// doesn't seed the per-file dev-doc store — so a plain PATCH 404s on first touch.
		ex = te.UpsertDevDocFile(ctx, store, extensionID, fileType, location, string(content))
		if ex == nil {
			dedup.Record(rel, content) // record only on success so failures retry
		}
	case "delete":
		ex = te.DeleteDevDocFile(ctx, store, extensionID, fileType, location)
		if ex == nil {
			dedup.Forget(rel)
		}
	}
	if ex != nil {
		fmt.Fprintf(stderr, "%s %s failed: %v\n", op, rel, ex)
		return
	}
	fmt.Fprintf(stdout, "synced (%s): %s\n", op, rel)
}

// printServeBanner prints te serve's two preview URLs in the same plain style as
// `themes serve`. The Admin (/admin/card) and Storefront (?preview_theme_id)
// URLs carry theme_id + ext_debug=<extensionID>. themeID is always present —
// PreRunE requires --theme-id.
func printServeBanner(w io.Writer, domain, extensionID, themeID string) {
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Admin preview — open in the Theme Editor:")
	fmt.Fprintf(w, "   https://%s/admin/card?theme_id=%s&ext_debug=%s\n", domain, themeID, extensionID)
	fmt.Fprintln(w)
	fmt.Fprintln(w, "Storefront preview — open in your browser:")
	fmt.Fprintf(w, "   https://%s?preview_theme_id=%s&ext_debug=%s\n", domain, themeID, extensionID)
	fmt.Fprintln(w)
}
