package pack

import (
	"archive/tar"
	"compress/gzip"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// TemplateTarballURL is the public, immutable Nova-2023 main-branch tarball.
const TemplateTarballURL = "https://codeload.github.com/Shoplazza/Nova-2023/tar.gz/refs/heads/main"

// templateTarballURL is a test override hook for httptest server URLs.
var templateTarballURL = TemplateTarballURL

const cloneTemplateMaxSize = 100 * 1024 * 1024 // 100 MB

// cloneTemplateTimeout caps the whole download+extract stream so a stalled
// connection cannot hang the command forever; the caller's ctx still aborts earlier.
const cloneTemplateTimeout = 60 * time.Second

// Typed sentinels for CloneTemplate failure classes; callers branch with errors.Is.
var (
	// ErrTargetDirNotEmpty rejects extraction into an existing non-empty directory.
	ErrTargetDirNotEmpty = errors.New("target directory already exists and is not empty")
	// ErrTemplateDownload flags a non-200 response from the template host.
	ErrTemplateDownload = errors.New("template download failed")
)

// CloneTemplate streams the Nova-2023 tarball from GitHub codeload and extracts
// it into targetDir, stripping the top-level "Shoplazza-Nova-2023-<sha>/" prefix.
// targetDir must be absent or an empty directory (ErrTargetDirNotEmpty otherwise).
func CloneTemplate(ctx context.Context, targetDir string) error {
	// Guard before any network or extraction work: extracting over a non-empty
	// directory would overwrite its files.
	if entries, err := os.ReadDir(targetDir); err == nil && len(entries) > 0 {
		return fmt.Errorf("%w: %s", ErrTargetDirNotEmpty, targetDir)
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodGet, templateTarballURL, nil)
	if err != nil {
		return fmt.Errorf("download template: %w", err)
	}
	httpClient := &http.Client{Timeout: cloneTemplateTimeout}
	resp, err := httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("download template: %w", err)
	}
	defer resp.Body.Close()
	if resp.StatusCode != 200 {
		return fmt.Errorf("%w: HTTP %d from %s", ErrTemplateDownload, resp.StatusCode, templateTarballURL)
	}
	gz, err := gzip.NewReader(resp.Body)
	if err != nil {
		return fmt.Errorf("gzip: %w", err)
	}
	defer gz.Close()
	tr := tar.NewReader(gz)

	if err := os.MkdirAll(targetDir, 0o755); err != nil {
		return err
	}
	absTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	var topPrefix string
	var total int64

	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			return fmt.Errorf("tar read: %w", err)
		}
		// Skip extended/global tar metadata entries; they have no path semantics
		// and would otherwise poison top-prefix detection (e.g. a leading pax_global_header).
		if hdr.Typeflag == tar.TypeXGlobalHeader || hdr.Typeflag == tar.TypeXHeader {
			continue
		}
		// Detect first-entry top prefix — only from real file/dir headers.
		if topPrefix == "" && (hdr.Typeflag == tar.TypeReg || hdr.Typeflag == tar.TypeDir) {
			parts := strings.SplitN(hdr.Name, "/", 2)
			if len(parts) > 0 && parts[0] != "" {
				topPrefix = parts[0] + "/"
			}
		}
		name := strings.TrimPrefix(hdr.Name, topPrefix)
		if name == "" {
			continue
		}
		for _, seg := range strings.Split(name, "/") {
			if seg == ".." {
				return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
			}
		}
		destPath := filepath.Join(absTarget, filepath.FromSlash(name))
		absDest, _ := filepath.Abs(destPath)
		if !strings.HasPrefix(absDest, absTarget+string(os.PathSeparator)) && absDest != absTarget {
			return fmt.Errorf("unsafe path in archive: %s", hdr.Name)
		}

		switch hdr.Typeflag {
		case tar.TypeDir:
			if err := os.MkdirAll(destPath, 0o755); err != nil {
				return err
			}
		case tar.TypeReg:
			if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
				return err
			}
			out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
			if err != nil {
				return err
			}
			remaining := int64(cloneTemplateMaxSize) - total
			if remaining <= 0 {
				out.Close()
				return fmt.Errorf("template archive too large (>%d bytes)", cloneTemplateMaxSize)
			}
			n, copyErr := io.Copy(out, io.LimitReader(tr, remaining+1))
			out.Close()
			if copyErr != nil {
				return copyErr
			}
			if n > remaining {
				return fmt.Errorf("template archive too large (>%d bytes)", cloneTemplateMaxSize)
			}
			total += n
		default:
			// symlink / fifo / char / block → ignored by design
			continue
		}
	}
	return nil
}
