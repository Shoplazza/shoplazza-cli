// Package binmgr downloads and caches external binaries (cloudflared, javy),
// keyed by name+version under the user cache dir.
package binmgr

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// classifiedError tags an ensureIn failure as network- or internal-class so
// Ensure can map it to the right exit code.
type classifiedError struct {
	network bool
	err     error
}

func (e *classifiedError) Error() string { return e.err.Error() }

// Spec describes one external binary: how to resolve its per-platform download
// URL and pinned integrity checksum.
type Spec struct {
	Name        string
	Version     string
	Compression string // "" = raw binary; "gzip" = gunzip after download; "tgz" = extract tar.gz entry named Name
	URL         func(goos, goarch string) (string, error)
	// SHA256 returns the lowercase-hex SHA-256 of the served artifact (before
	// decompression) for the target platform. When non-nil, the download is
	// verified before being cached; a missing/empty checksum is a hard error
	// (fail-closed). A nil func disables verification (test-only).
	SHA256 func(goos, goarch string) (string, error)
	// Progress, when non-nil, reports the download step on a cache miss. nil disables reporting.
	Progress *output.Progress
}

// Ensure returns the path to the binary, downloading+caching it under the user
// cache dir if not already present. Safe to call repeatedly.
func Ensure(ctx context.Context, spec Spec) (string, *output.ExitError) {
	root, err := cacheRoot()
	if err != nil {
		return "", output.ErrInternal("cannot resolve cache dir: %s", err.Error())
	}
	p, ierr := ensureIn(ctx, root, spec)
	if ierr != nil {
		var ce *classifiedError
		if errors.As(ierr, &ce) && !ce.network {
			return "", output.ErrInternal("failed to fetch %s %s: %s", spec.Name, spec.Version, ierr.Error())
		}
		return "", output.ErrNetwork("failed to fetch %s %s: %s", spec.Name, spec.Version, ierr.Error())
	}
	return p, nil
}

func cacheRoot() (string, error) {
	dir, err := os.UserCacheDir()
	if err != nil {
		return "", err
	}
	return filepath.Join(dir, "shoplazza-cli", "bin"), nil
}

func binName(name string) string {
	if runtime.GOOS == "windows" {
		return name + ".exe"
	}
	return name
}

// ensureIn is the testable core: download into <root>/<name>-<version>/<bin>.
func ensureIn(ctx context.Context, root string, spec Spec) (string, error) {
	dir := filepath.Join(root, spec.Name+"-"+spec.Version)
	dest := filepath.Join(dir, binName(spec.Name))
	if fi, err := os.Stat(dest); err == nil && fi.Mode().IsRegular() {
		return dest, nil // cache hit
	}
	url, err := spec.URL(runtime.GOOS, runtime.GOARCH)
	if err != nil {
		return "", &classifiedError{network: false, err: err}
	}
	// Resolve the pinned checksum (fail-closed): a missing/empty hash is a hard error.
	var wantSHA string
	if spec.SHA256 != nil {
		s, sErr := spec.SHA256(runtime.GOOS, runtime.GOARCH)
		if sErr != nil {
			return "", &classifiedError{network: false, err: sErr}
		}
		if s == "" {
			return "", &classifiedError{network: false, err: fmt.Errorf(
				"no pinned sha256 for %s %s on %s/%s", spec.Name, spec.Version, runtime.GOOS, runtime.GOARCH)}
		}
		wantSHA = s
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", &classifiedError{network: false, err: err}
	}
	// Report download and extraction as a single timed step.
	dl := spec.Progress.Begin("Downloading " + spec.Name)
	if err := download(ctx, url, dest, spec.Compression, spec.Name, wantSHA); err != nil {
		dl.Fail()
		return "", err // download already wraps errors with classifiedError
	}
	dl.Done()
	if err := os.Chmod(dest, 0o755); err != nil {
		return "", &classifiedError{network: false, err: err}
	}
	return dest, nil
}

// download fetches url, verifies the served artifact against wantSHA (when
// non-empty), then writes the binary to dest. compression selects the
// post-verify transform: "" copies verbatim, "gzip" gunzips, "tgz" extracts the
// tar.gz entry whose base name equals binName (required, else internal error).
// The hash is taken on the artifact as served, before any decompression, so the
// body is streamed verbatim to a temp file and only decompressed once it matches.
func download(ctx context.Context, url, dest, compression, binName, wantSHA string) error {
	cctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
	defer cancel()
	req, err := http.NewRequestWithContext(cctx, http.MethodGet, url, nil)
	if err != nil {
		return &classifiedError{network: true, err: err}
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &classifiedError{network: true, err: err}
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return &classifiedError{network: true, err: &httpStatusError{code: resp.StatusCode}}
	}

	// 1. Stream the served artifact verbatim to a temp file, hashing as it lands.
	raw := dest + ".raw"
	rf, err := os.Create(raw)
	if err != nil {
		return &classifiedError{network: false, err: err}
	}
	h := sha256.New()
	if _, err := io.Copy(io.MultiWriter(rf, h), resp.Body); err != nil {
		_ = rf.Close()
		_ = os.Remove(raw)
		return &classifiedError{network: true, err: err}
	}
	if err := rf.Close(); err != nil {
		_ = os.Remove(raw)
		return &classifiedError{network: false, err: err}
	}
	defer os.Remove(raw)

	// 2. Integrity gate: verify the served artifact before we decompress/exec it.
	if wantSHA != "" {
		if got := hex.EncodeToString(h.Sum(nil)); !strings.EqualFold(got, wantSHA) {
			return &classifiedError{network: false, err: fmt.Errorf(
				"integrity check failed for %s: got sha256 %s, want %s", binName, got, wantSHA)}
		}
	}

	// 3. Transform the verified artifact into the final binary.
	src, err := os.Open(raw)
	if err != nil {
		return &classifiedError{network: false, err: err}
	}
	defer src.Close()
	tmp := dest + ".part"
	f, err := os.Create(tmp)
	if err != nil {
		return &classifiedError{network: false, err: err}
	}

	var copyErr error
	switch compression {
	case "gzip":
		gr, gzErr := gzip.NewReader(src)
		if gzErr != nil {
			_ = f.Close()
			_ = os.Remove(tmp)
			return &classifiedError{network: false, err: gzErr}
		}
		_, copyErr = io.Copy(f, gr) //nolint:gosec // verified pinned release asset
		_ = gr.Close()
	case "tgz":
		copyErr = extractTgzEntry(src, f, binName)
	default:
		_, copyErr = io.Copy(f, src)
	}
	if copyErr != nil {
		_ = f.Close()
		_ = os.Remove(tmp)
		return &classifiedError{network: false, err: copyErr}
	}

	if err := f.Close(); err != nil {
		return &classifiedError{network: false, err: err}
	}
	if err := os.Rename(tmp, dest); err != nil {
		return &classifiedError{network: false, err: err}
	}
	return nil
}

// extractTgzEntry reads a gzip-compressed tar stream from r and copies the first
// regular-file entry whose base name equals binName into w. Returns an error if
// no such entry exists.
func extractTgzEntry(r io.Reader, w io.Writer, binName string) error {
	gr, err := gzip.NewReader(r)
	if err != nil {
		return err
	}
	defer gr.Close()
	tr := tar.NewReader(gr)
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			return fmt.Errorf("entry %q not found in tarball", binName)
		}
		if err != nil {
			return err
		}
		if hdr.Typeflag != tar.TypeReg {
			continue
		}
		if filepath.Base(hdr.Name) != binName {
			continue
		}
		if _, err := io.Copy(w, tr); err != nil { //nolint:gosec // pinned upstream release asset
			return err
		}
		return nil
	}
}

type httpStatusError struct{ code int }

func (e *httpStatusError) Error() string {
	return "download failed with status " + http.StatusText(e.code)
}
