package javy_test

import (
	"context"
	"crypto/md5"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/extbuild/javy"
)

// writeFakeJavy writes a shell script (or .bat on Windows) to dir that acts
// as a fake javy binary: reads args "build <entry> -o <out>", writes
// "fake-wasm" to <out>, and prints nothing to stderr.
func writeFakeJavy(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := `@echo off
set entry=%2
set out=%4
echo fake-wasm> %out%
`
		p := filepath.Join(dir, "javy.bat")
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	script := `#!/bin/sh
# fake javy: build <entry> -o <out>
# args: $1=build $2=<entry> $3=-o $4=<out>
printf 'fake-wasm' > "$4"
`
	p := filepath.Join(dir, "javy")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

// writeFakeJavyStderr writes a fake javy that writes to stderr and exits 0.
func writeFakeJavyStderr(t *testing.T, dir string) string {
	t.Helper()
	if runtime.GOOS == "windows" {
		script := `@echo off
set out=%4
echo fake-wasm> %out%
echo "javy warning: something went wrong" >&2
`
		p := filepath.Join(dir, "javy-stderr.bat")
		if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
			t.Fatal(err)
		}
		return p
	}
	script := `#!/bin/sh
printf 'fake-wasm' > "$4"
printf 'javy warning: something went wrong' >&2
`
	p := filepath.Join(dir, "javy-stderr")
	if err := os.WriteFile(p, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestBuild_RunsJavyAndNamesByHash(t *testing.T) {
	tmpDir := t.TempDir()
	fakeJavyPath := writeFakeJavy(t, tmpDir)

	// Write entry file with known content.
	const entryContent = "console.log('hello');\n"
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(srcDir, "index.js")
	if err := os.WriteFile(entryFile, []byte(entryContent), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "out")

	// Compute expected md5 of entry content.
	sum := md5.Sum([]byte(entryContent))
	expectedHash := fmt.Sprintf("%x", sum)
	expectedFileName := "myfn." + expectedHash + ".wasm"
	expectedPath := filepath.Join(outDir, expectedFileName)

	gotPath, xerr := javy.Build(context.Background(), fakeJavyPath, entryFile, outDir, "myfn")
	if xerr != nil {
		t.Fatalf("Build returned error: %v", xerr)
	}
	if gotPath != expectedPath {
		t.Fatalf("path = %q, want %q", gotPath, expectedPath)
	}
	if _, err := os.Stat(gotPath); err != nil {
		t.Fatalf("output file not present: %v", err)
	}
	content, err := os.ReadFile(gotPath)
	if err != nil {
		t.Fatalf("ReadFile: %v", err)
	}
	// TrimSuffix: the Windows .bat shim's `echo` appends a CRLF the POSIX shim
	// doesn't; anything else must still fail exact comparison.
	if got := strings.TrimSuffix(string(content), "\r\n"); got != "fake-wasm" {
		t.Fatalf("file content = %q, want %q", got, "fake-wasm")
	}
}

func TestBuild_StderrIsFailure(t *testing.T) {
	tmpDir := t.TempDir()
	fakeJavyPath := writeFakeJavyStderr(t, tmpDir)

	const entryContent = "console.log('hello');\n"
	srcDir := filepath.Join(tmpDir, "src")
	if err := os.MkdirAll(srcDir, 0o755); err != nil {
		t.Fatal(err)
	}
	entryFile := filepath.Join(srcDir, "index.js")
	if err := os.WriteFile(entryFile, []byte(entryContent), 0o644); err != nil {
		t.Fatal(err)
	}

	outDir := filepath.Join(tmpDir, "out")

	gotPath, xerr := javy.Build(context.Background(), fakeJavyPath, entryFile, outDir, "myfn")
	if xerr == nil {
		t.Fatal("expected ExitError for stderr output, got nil")
	}
	if gotPath != "" {
		t.Fatalf("expected empty path on failure, got %q", gotPath)
	}
}

func TestBuild_MissingEntry(t *testing.T) {
	tmpDir := t.TempDir()
	fakeJavyPath := writeFakeJavy(t, tmpDir)

	nonExistent := filepath.Join(tmpDir, "does-not-exist.js")
	outDir := filepath.Join(tmpDir, "out")

	gotPath, xerr := javy.Build(context.Background(), fakeJavyPath, nonExistent, outDir, "myfn")
	if xerr == nil {
		t.Fatal("expected ExitError for missing entry, got nil")
	}
	if gotPath != "" {
		t.Fatalf("expected empty path on failure, got %q", gotPath)
	}
	// Must be a validation error.
	if xerr.Detail == nil || xerr.Detail.Type != "validation" {
		t.Fatalf("expected validation error, got: %+v", xerr)
	}
}

func TestSpec_URLMapping(t *testing.T) {
	spec := javy.Spec()

	// Check a known good mapping.
	url, err := spec.URL("darwin", "arm64")
	if err != nil {
		t.Fatalf("URL(darwin,arm64): %v", err)
	}
	want := "https://github.com/bytecodealliance/javy/releases/download/v5.0.1/javy-arm-macos-v5.0.1.gz"
	if url != want {
		t.Fatalf("URL = %q, want %q", url, want)
	}

	// Check all valid mappings contain the expected archPlatform token.
	cases := []struct {
		goos, goarch string
		token        string
	}{
		{"darwin", "arm64", "arm-macos"},
		{"darwin", "amd64", "x86_64-macos"},
		{"linux", "amd64", "x86_64-linux"},
		{"linux", "arm64", "arm-linux"},
		{"windows", "amd64", "x86_64-windows"},
	}
	for _, c := range cases {
		u, e := spec.URL(c.goos, c.goarch)
		if e != nil {
			t.Errorf("URL(%s,%s): unexpected error: %v", c.goos, c.goarch, e)
			continue
		}
		if !strings.Contains(u, c.token) {
			t.Errorf("URL(%s,%s) = %q, want token %q", c.goos, c.goarch, u, c.token)
		}
	}

	// Unsupported combo must return an error.
	_, err = spec.URL("plan9", "mips")
	if err == nil {
		t.Fatal("expected error for unsupported platform plan9/mips, got nil")
	}
}

// TestSpec_SHA256_CoversEveryURLPlatform guards against drift: every URL-supported
// platform must have a non-empty pinned checksum, and unsupported platforms must be
// rejected symmetrically.
func TestSpec_SHA256_CoversEveryURLPlatform(t *testing.T) {
	spec := javy.Spec()
	cases := []struct{ goos, goarch string }{
		{"darwin", "arm64"}, {"darwin", "amd64"},
		{"linux", "amd64"}, {"linux", "arm64"},
		{"windows", "amd64"},
		{"plan9", "mips"}, {"linux", "riscv64"}, // unsupported
	}
	for _, c := range cases {
		_, urlErr := spec.URL(c.goos, c.goarch)
		sum, shaErr := spec.SHA256(c.goos, c.goarch)
		if urlErr == nil {
			if shaErr != nil {
				t.Errorf("URL supports %s/%s but SHA256 errored: %v", c.goos, c.goarch, shaErr)
			}
			if sum == "" {
				t.Errorf("URL supports %s/%s but SHA256 is empty (silent-skip risk)", c.goos, c.goarch)
			}
		} else if shaErr == nil {
			t.Errorf("URL rejects %s/%s but SHA256 returned %q (inconsistent)", c.goos, c.goarch, sum)
		}
	}
}
