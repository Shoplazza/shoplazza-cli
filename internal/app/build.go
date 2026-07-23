package app

import (
	"archive/zip"
	"context"
	"crypto/md5"
	"encoding/hex"
	"fmt"
	"io"
	"io/fs"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/extbuild/javy"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/jsbuild"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// themeZipName ports v1's buildTheme.js filename: "<name>-<hash8><ts8>.zip",
// where hash8 is the first 8 hex of an md5 over the source dir's file contents
// (sorted for determinism, .git excluded) and ts8 is 8 hex of the current time.
// The name MUST be content/time-unique: the OSS upload sends x-oss-forbid-
// overwrite and a 409 is swallowed (the old object's URL is returned), so a
// static "<name>.zip" would silently reuse a STALE artifact on every re-deploy.
func themeZipName(srcDir, name string) (string, error) {
	var files []string
	if err := filepath.WalkDir(srcDir, func(p string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		files = append(files, p)
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
	return fmt.Sprintf("%s-%s%s.zip", name, sum[:8], ts), nil
}

// zipExtension packs srcDir into a zip at outPath (creating parent dirs),
// excluding any .git directory, with forward-slash relative paths. Returns
// outPath. Used by the theme deploy leg.
//
// topDir, when non-empty, prefixes every entry with "<topDir>/" so the archive
// has a single top-level directory. The theme leg passes "theme-app" to match
// v1 (buildTheme.js zips with adm-zip's rename:"theme-app"); the backend's theme
// version task resolves assets/ relative to that wrapper, so omitting it makes
// the task fail with "assets/assets-manifest.json: no such file".
func zipExtension(srcDir, outPath, topDir string) (string, error) {
	if err := os.MkdirAll(filepath.Dir(outPath), 0o755); err != nil {
		return "", err
	}
	f, err := os.Create(outPath)
	if err != nil {
		return "", err
	}
	// Backstop for the error paths only — the success path closes explicitly
	// below and checks the error, so a failed flush can't ship a corrupt zip.
	defer f.Close()
	zw := zip.NewWriter(f)

	walkErr := filepath.WalkDir(srcDir, func(path string, d fs.DirEntry, err error) error {
		if err != nil {
			return err
		}
		if d.IsDir() {
			if d.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, err := filepath.Rel(srcDir, path)
		if err != nil {
			return err
		}
		name := filepath.ToSlash(rel)
		if topDir != "" {
			name = topDir + "/" + name
		}
		w, err := zw.Create(name)
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
	if err := f.Close(); err != nil {
		return "", err
	}
	return outPath, nil
}

// buildCheckout builds a checkout extension via the Vite bridge (reused
// internal/jsbuild) and returns the absolute artifact path.
// extDirName is the local extension directory name under extensions/.
// Artifacts[0] is relative to projectCwd (same convention as cmd/checkout/push.go:52).
func buildCheckout(ctx context.Context, projectCwd, extDirName string, debug bool) (string, *output.ExitError) {
	res, exErr := jsbuild.RunBuild(ctx, jsbuild.BuildRequest{
		Action: "build",
		Name:   extDirName,
		Debug:  debug,
	}, projectCwd)
	if exErr != nil {
		return "", exErr
	}
	if res == nil || len(res.Artifacts) == 0 {
		return "", output.ErrInternal("checkout build produced no artifacts for %q", extDirName)
	}
	return filepath.Join(projectCwd, res.Artifacts[0]), nil
}

// BuildArtifactFor produces a deployable artifact for a single local extension,
// dispatching by type. It is the real BuildArtifact wired by `app deploy`:
//   - checkout:  Vite build via buildCheckout, returns the bundle path.
//   - theme:     zip extensions/<Dir> into app-deploy/<Name>.zip, returns its path.
//   - function:  javy-compile extensions/<Dir>/src/index.js to app-deploy/<Name>.<hash>.wasm.
func BuildArtifactFor(ctx context.Context, projectRoot string, l LocalExt, debug bool) (string, *output.ExitError) {
	switch l.Type {
	case "checkout":
		return buildCheckout(ctx, projectRoot, l.Dir, debug)
	case "theme":
		src := filepath.Join(projectRoot, "extensions", l.Dir)
		// v1 nests theme content under theme-app/; zip that subdir so the archive
		// isn't double-nested (theme-app/theme-app/...). v2 (flat) has no such subdir.
		if sub := filepath.Join(src, "theme-app"); isDir(sub) {
			src = sub
		}
		// Content/time-unique name (v1 parity) — a static name collides with the
		// overwrite-forbidden OSS object and silently reuses a stale artifact.
		zipName, nErr := themeZipName(src, l.Name)
		if nErr != nil {
			return "", output.ErrInternal("hash theme extension %q: %v", l.Dir, nErr)
		}
		out := filepath.Join(projectRoot, "app-deploy", zipName)
		// "theme-app" wrapper dir — v1 parity (buildTheme.js rename:"theme-app").
		zipped, err := zipExtension(src, out, "theme-app")
		if err != nil {
			return "", output.ErrInternal("zip theme extension %q: %v", l.Dir, err)
		}
		return zipped, nil
	case "function":
		javyPath, jErr := javy.Ensure(ctx)
		if jErr != nil {
			return "", jErr
		}
		entry := functionEntryPath(projectRoot, l.Dir)
		outDir := filepath.Join(projectRoot, "app-deploy")
		return javy.Build(ctx, javyPath, entry, outDir, l.Name)
	default:
		return "", output.ErrValidation("unknown extension type %q in %s", l.Type, l.Dir)
	}
}

// isDir reports whether path exists and is a directory.
func isDir(path string) bool {
	fi, err := os.Stat(path)
	return err == nil && fi.IsDir()
}

// functionEntryPath returns the JS entry for a function extension:
// <projectRoot>/extensions/<dir>/src/index.js. Single source of the layout
// convention shared by the javy build (build.go) and the source_code upload (deploy.go).
func functionEntryPath(projectRoot, dir string) string {
	return filepath.Join(projectRoot, "extensions", dir, "src", "index.js")
}
