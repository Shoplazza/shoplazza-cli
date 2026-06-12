package pack

import (
	"archive/zip"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"sort"
	"strings"
)

// ThemeDirs lists the 8 standard top-level directories a Shoplazza theme has.
var ThemeDirs = []string{
	"assets", "blocks", "config", "layout",
	"locales", "sections", "snippets", "templates",
}

// Typed sentinels for Unpack failure classes; callers branch with errors.Is.
var (
	// ErrUnsafeArchivePath flags a zip entry that would escape the target directory.
	// The wrapping message names the offending entry, not the archive file.
	ErrUnsafeArchivePath = errors.New("unsafe path in archive")
	// ErrSizeLimit flags an archive whose cumulative extracted bytes exceed the limit.
	ErrSizeLimit = errors.New("extracted size limit exceeded")
)

var themeDirSet = func() map[string]struct{} {
	m := make(map[string]struct{}, len(ThemeDirs))
	for _, d := range ThemeDirs {
		m[d] = struct{}{}
	}
	return m
}()

// PackOptions controls Pack behavior.
type PackOptions struct {
	IncludeHidden bool
	// IgnoreFile is the path to a gitignore-syntax ignore file.
	//   "" (empty)              → auto-detect srcDir/.themeignore if present
	//   explicit path           → use that file
	//   "/dev/null" or missing  → force-disable all ignore filtering
	IgnoreFile string
}

// UnpackOptions controls Unpack behavior.
type UnpackOptions struct {
	StripTopDir        bool  // If true, strip the first path segment of every entry.
	MaxTotalSize       int64 // Cumulative extracted bytes limit; 0 = default 200 MB.
	PathTraversalCheck bool  // If true, reject entries that escape targetDir.
}

const defaultMaxUnpackSize = 200 * 1024 * 1024

// Pack writes a zip archive containing all files under srcDir that belong to a
// theme directory (per ThemeDirs) and are not excluded by .themeignore.
// Returns the absolute path to the produced zip.
func Pack(srcDir, outputName string, opts PackOptions) (string, error) {
	zipPath := outputName
	if !filepath.IsAbs(zipPath) {
		abs, err := filepath.Abs(zipPath)
		if err == nil {
			zipPath = abs
		}
	}
	f, err := os.Create(zipPath)
	if err != nil {
		return "", fmt.Errorf("create zip: %w", err)
	}
	// Error-path backstops only; the success path closes explicitly below so
	// flush/close failures propagate instead of producing a bad zip.
	defer f.Close()
	zw := zip.NewWriter(f)
	defer zw.Close()

	ignorer, err := loadIgnorer(srcDir, opts.IgnoreFile)
	if err != nil {
		return "", err
	}

	rels, err := EnumerateThemeFiles(srcDir)
	if err != nil {
		return "", err
	}
	sort.Strings(rels)

	for _, rel := range rels {
		if rel == ".themeignore" {
			continue
		}
		if ignorer != nil && ignorer.MatchesPath(rel) {
			continue
		}
		if !opts.IncludeHidden && isHidden(rel) {
			continue
		}
		full := filepath.Join(srcDir, filepath.FromSlash(rel))
		info, err := os.Stat(full)
		if err != nil {
			return "", fmt.Errorf("stat %s: %w", rel, err)
		}
		hdr, err := zip.FileInfoHeader(info)
		if err != nil {
			return "", err
		}
		hdr.Name = rel // forward-slash relative path; required for cross-OS zip readers
		hdr.Method = zip.Deflate
		w, err := zw.CreateHeader(hdr)
		if err != nil {
			return "", err
		}
		src, err := os.Open(full)
		if err != nil {
			return "", err
		}
		if _, err := io.Copy(w, src); err != nil {
			src.Close()
			return "", err
		}
		src.Close()
	}
	// Explicit close with error propagation: writing the central directory or
	// flushing to disk can fail and must fail the Pack.
	if err := zw.Close(); err != nil {
		return "", fmt.Errorf("finalize zip: %w", err)
	}
	if err := f.Close(); err != nil {
		return "", fmt.Errorf("close zip: %w", err)
	}
	return zipPath, nil
}

// EnumerateThemeFiles returns the list of forward-slash relative paths under
// srcDir that fall inside any of the 8 standard ThemeDirs.
func EnumerateThemeFiles(srcDir string) ([]string, error) {
	var out []string
	for _, dir := range ThemeDirs {
		base := filepath.Join(srcDir, dir)
		if _, err := os.Stat(base); errors.Is(err, os.ErrNotExist) {
			continue
		}
		err := filepath.Walk(base, func(p string, info os.FileInfo, walkErr error) error {
			if walkErr != nil {
				return walkErr
			}
			if info.IsDir() {
				return nil
			}
			rel, err := filepath.Rel(srcDir, p)
			if err != nil {
				return err
			}
			out = append(out, filepath.ToSlash(rel))
			return nil
		})
		if err != nil {
			return nil, err
		}
	}
	// Always include .themeignore in the scan so callers can decide; Pack itself excludes.
	ti := filepath.Join(srcDir, ".themeignore")
	if _, err := os.Stat(ti); err == nil {
		out = append(out, ".themeignore")
	}
	return out, nil
}

func isHidden(rel string) bool {
	for _, segment := range strings.Split(rel, "/") {
		if strings.HasPrefix(segment, ".") && segment != "." && segment != ".themeignore" {
			return true
		}
	}
	return false
}

// Unpack extracts zipPath into targetDir. See UnpackOptions for semantics.
// Existing files are always overwritten.
func Unpack(zipPath, targetDir string, opts UnpackOptions) error {
	maxSize := opts.MaxTotalSize
	if maxSize <= 0 {
		maxSize = defaultMaxUnpackSize
	}
	r, err := zip.OpenReader(zipPath)
	if err != nil {
		return fmt.Errorf("open zip: %w", err)
	}
	defer r.Close()

	cleanTarget, err := filepath.Abs(targetDir)
	if err != nil {
		return err
	}

	// Compute top dir to strip (first segment of first entry that has one).
	var stripPrefix string
	if opts.StripTopDir {
		for _, f := range r.File {
			parts := strings.SplitN(f.Name, "/", 2)
			if len(parts) > 1 && parts[0] != "" {
				stripPrefix = parts[0] + "/"
				break
			}
		}
	}

	var totalWritten int64
	for _, f := range r.File {
		name := f.Name
		if stripPrefix != "" {
			if name == strings.TrimSuffix(stripPrefix, "/") || name == stripPrefix {
				continue
			}
			name = strings.TrimPrefix(name, stripPrefix)
			if name == "" {
				continue
			}
		}
		// Segment-level path traversal check: rejects ".." components without
		// matching legit names like "..foo".
		if opts.PathTraversalCheck {
			for _, seg := range strings.Split(name, "/") {
				if seg == ".." {
					return fmt.Errorf("%w: %s", ErrUnsafeArchivePath, f.Name)
				}
			}
		}
		destPath := filepath.Join(cleanTarget, filepath.FromSlash(name))
		// Defensive: also check absolute path stays within cleanTarget
		absDest, _ := filepath.Abs(destPath)
		if !strings.HasPrefix(absDest, cleanTarget+string(os.PathSeparator)) && absDest != cleanTarget {
			return fmt.Errorf("%w: %s", ErrUnsafeArchivePath, f.Name)
		}

		if f.FileInfo().IsDir() {
			_ = os.MkdirAll(destPath, 0o755)
			continue
		}
		if err := os.MkdirAll(filepath.Dir(destPath), 0o755); err != nil {
			return err
		}
		rc, err := f.Open()
		if err != nil {
			return fmt.Errorf("open entry %s: %w", f.Name, err)
		}
		out, err := os.OpenFile(destPath, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0o644)
		if err != nil {
			rc.Close()
			return err
		}
		// Limit-aware copy.
		remaining := maxSize - totalWritten
		if remaining <= 0 {
			out.Close()
			rc.Close()
			return fmt.Errorf("%w: theme archive exceeds %d bytes (at entry %s)", ErrSizeLimit, maxSize, f.Name)
		}
		n, copyErr := io.Copy(out, io.LimitReader(rc, remaining+1))
		out.Close()
		rc.Close()
		if copyErr != nil {
			return copyErr
		}
		if n > remaining {
			return fmt.Errorf("%w: theme archive exceeds %d bytes (at entry %s)", ErrSizeLimit, maxSize, f.Name)
		}
		totalWritten += n
	}
	return nil
}
