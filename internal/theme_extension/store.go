package theme_extension

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/app"
	"github.com/Shoplazza/shoplazza-cli/internal/client"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// Register performs the no-connect registration for a te project: it reuses
// app.RegisterThemeExtension (PUT theme-extensions + version-tasks + poll) — the
// same store-openapi machinery app deploy's theme leg uses — then writes the
// resulting extension_id back to extension.config.json (truth source).
// create-vs-commit is decided by the config's existing extension_id, not a
// partner-context query (te is store-side, pre-connect). It never connects.
//
// version is the per-build semver: `te serve` passes "" (create at 1.0.0 on a
// fresh extension, no bump on re-register); `te build` passes the user's
// --version, honored on both the create and commit path (v1 te accepts any
// valid semver even on the first build). resourceURL is the OSS url from a prior
// ossupload.Upload of the zipped theme-app/ directory.
func Register(ctx context.Context, store *client.Client, root, name, resourceURL, version, description string, pollInterval time.Duration, maxRetry int) (app.UpsertResult, *output.ExitError) {
	cfg, err := ReadConfig(root)
	if err != nil {
		return app.UpsertResult{}, output.ErrInternal("read te config: %v", err)
	}
	// ExtensionVersion is set unconditionally so the user's --version reaches the
	// create path too (RegisterThemeExtension honors a non-empty version on create).
	// ExtensionID is set only when we already have one (commit). Exts carries
	// `te build --description` to the version-task body (v1 field name); "" for
	// serve, which omits it (no formal version).
	ext := app.Extension{
		ExtensionName:    name,
		ExtensionType:    "theme",
		ResourceURL:      resourceURL,
		ExtensionVersion: version, // "" for serve → defaults to 1.0.0 inside RegisterThemeExtension
		Exts:             description,
	}
	if cfg.ExtensionID != "" {
		ext.ExtensionID = cfg.ExtensionID // commit path
	}
	res, exErr := app.RegisterThemeExtension(ctx, ext, store, pollInterval, maxRetry)
	if exErr != nil {
		return app.UpsertResult{}, exErr
	}
	// Load-bearing write-back: persist the id atomically (truth source).
	cfg.ExtensionID = res.ExtensionID
	// Record the built version so `te deploy` can default to it. `te build` passes
	// a non-empty version; `te serve` passes "" and must NOT clobber a previously
	// recorded version.
	if version != "" {
		cfg.Version = version
	}
	if err := WriteConfig(root, cfg); err != nil {
		return app.UpsertResult{}, output.ErrInternal("register succeeded but failed to persist extension_id: %v", err)
	}
	return res, nil
}

// ErrThemeAppMissing marks the only user-fixable ZipThemeApp failure: there is
// no theme-app/ directory under the project root. Callers branch with
// errors.Is — this case is validation, everything else (perms, read errors,
// zip writes) is internal.
var ErrThemeAppMissing = errors.New("theme-app is missing or not a directory")

// ZipThemeApp zips <root>/theme-app/ including the "theme-app/" wrapper directory
// as the entry prefix (e.g. "theme-app/blocks/x.liquid"), matching v1's compress.
// The backend's version-task / doctree parser requires this wrapper: it unzips
// and reads files under "theme-app/…" (e.g. theme-app/assets-manifest.json). A
// flattened zip (entries at the root) makes the parser build no doc — the version
// is created but doc-less, and a later `te release` fails with "version has no
// doc". Returns the zip path under <root>/.te-build/.
func ZipThemeApp(root string) (string, error) {
	srcDir := filepath.Join(root, "theme-app")
	fi, statErr := os.Stat(srcDir)
	switch {
	case os.IsNotExist(statErr):
		return "", fmt.Errorf("%w: %v", ErrThemeAppMissing, statErr)
	case statErr != nil:
		return "", statErr // EACCES etc. — environment trouble, not a layout problem
	case !fi.IsDir(): // exists but is a regular file → invalid te project layout
		return "", ErrThemeAppMissing
	}
	outDir := filepath.Join(root, ".te-build")
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", err
	}
	// Content+time-unique name → distinct OSS key per build. The OSS sign endpoint
	// sets x-oss-forbid-overwrite, so a static "theme-app.zip" makes every build
	// after the first reuse the stale object.
	zipName, nerr := themeAppZipName(srcDir)
	if nerr != nil {
		return "", nerr
	}
	outPath := filepath.Join(outDir, zipName)
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	defer f.Close()
	// Prefix every entry with the "theme-app/" wrapper dir (see the function doc).
	wrapper := filepath.Base(srcDir) // "theme-app"
	zw := zip.NewWriter(f)
	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() {
			return err
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		w, err := zw.Create(wrapper + "/" + filepath.ToSlash(rel))
		if err != nil {
			return err
		}
		src, err := os.Open(path)
		if err != nil {
			return err
		}
		defer src.Close()
		_, err = io.Copy(w, src)
		return err
	})
	if walkErr != nil {
		_ = zw.Close()
		return "", walkErr
	}
	if err := zw.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

// themeAppZipName builds a content+time-unique zip filename ("theme-app-<md5_8><ts_8>.zip")
// so each build uploads to a distinct OSS key (ossupload keys on the basename), avoiding
// the x-oss-forbid-overwrite stale-object reuse a static name causes.
func themeAppZipName(srcDir string) (string, error) {
	var files []string
	if err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if !d.IsDir() {
			files = append(files, p)
		}
		return nil
	}); err != nil {
		return "", err
	}
	sort.Strings(files)
	h := md5.New()
	for _, p := range files {
		b, err := os.ReadFile(p)
		if err != nil {
			return "", err
		}
		h.Write(b)
	}
	sum := hex.EncodeToString(h.Sum(nil))
	ts := strconv.FormatInt(time.Now().UnixNano(), 16)
	if len(ts) > 8 {
		ts = ts[len(ts)-8:]
	}
	return fmt.Sprintf("theme-app-%s%s.zip", sum[:8], ts), nil
}

// PushDevDoctree pushes the full dev tree (PATCH /theme-extensions/{id}/dev-doctree)
// — a dev-state push, not a formal version (no version explosion via
// POST /version-tasks). resourceURL is the OSS url of the freshly zipped
// theme-app/.
//
// The push is async: the PATCH response carries {task_id}. We poll that task to
// completion at GET /version-tasks/{id} (state 1 = done, 2 = failed) — the same
// task-status endpoint v1's serve polls. Polling at that path does not mint a
// formal version (that requires a POST to /version-tasks). pollInterval/maxRetry
// bound the wait. A response without task_id is treated as a synchronous 2xx
// success (defensive; live data always carries one).
func PushDevDoctree(ctx context.Context, store *client.Client, extensionID, resourceURL string, pollInterval time.Duration, maxRetry int) *output.ExitError {
	var resp struct {
		TaskID string `json:"task_id"`
	}
	body := map[string]any{"resource_url": resourceURL}
	if err := store.PatchJSON(ctx, "/openapi/2020-07/theme-extensions/"+extensionID+"/dev-doctree", body, &resp); err != nil {
		return apiOrInternalTE(err)
	}
	if resp.TaskID == "" {
		return nil
	}
	// Reuse the shared version-task poller (same GET /version-tasks/{id} endpoint
	// and state semantics); the returned version_id is empty for a dev push and
	// intentionally ignored.
	if _, err := app.PollThemeVersionTask(ctx, store, extensionID, resp.TaskID, pollInterval, maxRetry); err != nil {
		return err
	}
	return nil
}

// devDocPath is the per-file dev-doc endpoint for an extension (incremental serve).
func devDocPath(extensionID string) string {
	return "/openapi/2020-07/theme-extensions/" + extensionID + "/dev-doc"
}

// DevDocTarget maps a forward-slash relative path under theme-app/ to the
// (type, location) the dev-doc endpoint expects, replicating v1 getFileInfo:
// type = the immediate parent directory name, location = the base file name.
func DevDocTarget(relSlash string) (fileType, location string) {
	return path.Base(path.Dir(relSlash)), path.Base(relSlash)
}

// CreateDevDocFile pushes a newly-added file (POST dev-doc {type,location,content}).
func CreateDevDocFile(ctx context.Context, store *client.Client, extensionID, fileType, location, content string) *output.ExitError {
	var out any
	body := map[string]any{"type": fileType, "location": location, "content": content}
	if err := store.PostJSON(ctx, devDocPath(extensionID), body, &out); err != nil {
		return apiOrInternalTE(err)
	}
	return nil
}

// UpdateDevDocFile pushes a changed file (PATCH dev-doc {type,location,content}).
func UpdateDevDocFile(ctx context.Context, store *client.Client, extensionID, fileType, location, content string) *output.ExitError {
	var out any
	body := map[string]any{"type": fileType, "location": location, "content": content}
	if err := store.PatchJSON(ctx, devDocPath(extensionID), body, &out); err != nil {
		return apiOrInternalTE(err)
	}
	return nil
}

// UpsertDevDocFile ensures a dev-doc file holds content: PATCH it, and if the
// per-file doc doesn't exist yet, fall back to POST-create. The doc can be
// absent on an "update" for two reasons: (1) the initial bulk dev-doctree push
// does not seed the per-file dev-doc store, so the first per-file touch finds
// nothing; (2) atomic-save editors fire remove+create+chmod, which the fsnotify
// watcher coalesces to a chmod and delivers as "update".
func UpsertDevDocFile(ctx context.Context, store *client.Client, extensionID, fileType, location, content string) *output.ExitError {
	var out any
	body := map[string]any{"type": fileType, "location": location, "content": content}
	err := store.PatchJSON(ctx, devDocPath(extensionID), body, &out)
	if err == nil {
		return nil
	}
	var he *client.HTTPError
	if errors.As(err, &he) && devDocMissing(he) {
		// POST carries content, so this both creates the doc and fills it.
		if cErr := store.PostJSON(ctx, devDocPath(extensionID), body, &out); cErr != nil {
			return apiOrInternalTE(cErr)
		}
		return nil
	}
	return apiOrInternalTE(err)
}

// devDocMissing reports whether an HTTP error means the dev-doc file does not
// exist yet (so the caller should create it). The backend returns
// "extension doc not found"; a 404 is also treated as missing. The substring
// match is confined to 4xx — a 5xx whose message happens to contain "not found"
// is a server fault, not a missing doc.
func devDocMissing(he *client.HTTPError) bool {
	if he.StatusCode == http.StatusNotFound {
		return true
	}
	return he.StatusCode >= 400 && he.StatusCode < 500 &&
		strings.Contains(strings.ToLower(he.Body), "not found")
}

// DeleteDevDocFile removes a file (DELETE dev-doc?type=&location=). v1 sends the
// target as query params (no body) on delete.
func DeleteDevDocFile(ctx context.Context, store *client.Client, extensionID, fileType, location string) *output.ExitError {
	var out any
	q := map[string]any{"type": fileType, "location": location}
	if err := store.DeleteJSONWithQuery(ctx, devDocPath(extensionID), q, &out); err != nil {
		return apiOrInternalTE(err)
	}
	return nil
}

// teDigToArray returns the first []any at body, body.data, or body.data.data.
func teDigToArray(v any) []any {
	if a, ok := v.([]any); ok {
		return a
	}
	m, ok := v.(map[string]any)
	if !ok {
		return nil
	}
	switch d := m["data"].(type) {
	case []any:
		return d
	case map[string]any:
		if a, ok := d["data"].([]any); ok {
			return a
		}
	}
	return nil
}

func asMaps(arr []any) []map[string]any {
	out := make([]map[string]any, 0, len(arr))
	for _, it := range arr {
		if m, ok := it.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

// ListVersions GETs /theme-extensions/{id}/versions (store-token).
func ListVersions(ctx context.Context, store *client.Client, extensionID string) ([]map[string]any, *output.ExitError) {
	var body any
	if err := store.GetJSON(ctx, "/openapi/2020-07/theme-extensions/"+extensionID+"/versions", &body); err != nil {
		return nil, apiOrInternalTE(err)
	}
	return asMaps(teDigToArray(body)), nil
}

// ListExtensions GETs /theme-extensions (store-token) — the store's private
// theme-extension list. Also the recovery path: a lost config can find its
// extension_id back here by title/name.
func ListExtensions(ctx context.Context, store *client.Client) ([]map[string]any, *output.ExitError) {
	var body any
	if err := store.GetJSON(ctx, "/openapi/2020-07/theme-extensions", &body); err != nil {
		return nil, apiOrInternalTE(err)
	}
	return asMaps(teDigToArray(body)), nil
}

const (
	// StorePublicationsPath is the store-openapi (2020-07) publications endpoint
	// (te deploy → store-visible). PartnerPublicationsPath is the partner-openapi
	// one (te release → app-visible). Same body, different base+token.
	// Note: publications lives at 2024-07; the 2025-06 partner-openapi has
	// /connection but not /publications (404).
	StorePublicationsPath   = "/openapi/2020-07/theme-extensions/publications"
	PartnerPublicationsPath = "/openapi/2024-07/theme-extensions/publications"
)

// pubResp is a local {code,message} envelope for the publications endpoint,
// defined here so the te package stays self-contained.
type pubResp struct {
	Code    json.RawMessage `json:"code"`
	Message string          `json:"message"`
}

// Publish POSTs {extension_id, version_id, type:"enable"} to a publications
// endpoint. The CALLER supplies the path AND a client already pointed at the
// right base with the right token (store for deploy, partner for release) — this
// function is base/token-agnostic so the two commands cannot diverge.
func Publish(ctx context.Context, c *client.Client, path, extensionID, versionID string) *output.ExitError {
	var res pubResp
	body := map[string]any{"extension_id": extensionID, "version_id": versionID, "type": "enable"}
	if err := c.PostJSON(ctx, path, body, &res); err != nil {
		return apiOrInternalTE(err)
	}
	// The endpoint reports business failures inside 2xx bodies (e.g. 200
	// {"code":"4001","message":"version not found"}). An absent code means the
	// {"code":"Success",data} envelope was unwrapped by the client → success;
	// any other surviving code is a real, server-reported failure.
	if len(res.Code) > 0 && !app.ConnectionSucceeded(res.Code) {
		msg := res.Message
		if msg == "" {
			msg = "publish theme extension version failed"
		}
		return output.Errorf(output.ExitAPI, output.TypeAPI,
			"theme extension %q publication failed: %s", extensionID, msg)
	}
	return nil
}

// apiOrInternalTE mirrors app.apiOrInternal for the te package (HTTP → api,
// naming the failing endpoint; transport-level net.Error → network; else internal).
func apiOrInternalTE(err error) *output.ExitError {
	var he *client.HTTPError
	if errors.As(err, &he) {
		return output.ErrAPI(he.StatusCode, he.Body, "").WithEndpoint(he.Method, he.Path)
	}
	var netErr net.Error
	if errors.As(err, &netErr) {
		return output.ErrNetwork("%v", err)
	}
	return output.ErrInternal("%v", err)
}
