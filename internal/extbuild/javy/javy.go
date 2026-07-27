// Package javy wraps the javy CLI to compile a function extension's JS entry to
// WASM: `javy build <entry> -o <out>`, output named <name>.<md5(entry)>.wasm,
// with non-empty stderr treated as failure.
package javy

import (
	"bytes"
	"context"
	"crypto/md5"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/binmgr"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

const (
	javyVersion = "v5.0.1"
	javyName    = "javy"
)

// Build compiles a JavaScript function extension entry file to WASM using javy,
// writing the output as <name>.<md5hex-of-entry-content>.wasm and returning its
// path. A missing entry yields a validation ExitError; any javy stderr (even on
// exit 0) or a non-zero exit yields an internal ExitError.
func Build(ctx context.Context, javyPath, entry, outDir, name string) (string, *output.ExitError) {
	// Validate entry file exists.
	if _, err := os.Stat(entry); err != nil {
		return "", output.ErrValidation("function entry not found: %s", entry)
	}

	// Compute md5 of entry file content.
	hash, err := md5File(entry)
	if err != nil {
		return "", output.ErrInternal("failed to hash entry file: %s", err)
	}

	// Determine output path.
	if err := os.MkdirAll(outDir, 0o755); err != nil {
		return "", output.ErrInternal("failed to create output dir: %s", err)
	}
	outPath := filepath.Join(outDir, name+"."+hash+".wasm")

	// Run javy.
	var stderrBuf bytes.Buffer
	cmd := exec.CommandContext(ctx, javyPath, "build", entry, "-o", outPath)
	cmd.Stderr = &stderrBuf

	if execErr := cmd.Run(); execErr != nil {
		if stderrBuf.Len() > 0 {
			return "", output.ErrInternal("javy build failed: %s: %s", execErr, strings.TrimSpace(stderrBuf.String()))
		}
		return "", output.ErrInternal("javy build failed: %s", execErr)
	}

	// Non-empty stderr counts as failure even with exit code 0.
	if stderrBuf.Len() > 0 {
		return "", output.ErrInternal("javy build failed: %s", stderrBuf.String())
	}

	return outPath, nil
}

// md5File computes the MD5 hex digest of the file at path by streaming it.
func md5File(path string) (string, error) {
	f, err := os.Open(path)
	if err != nil {
		return "", err
	}
	defer f.Close()
	h := md5.New()
	if _, err := io.Copy(h, f); err != nil {
		return "", err
	}
	return fmt.Sprintf("%x", h.Sum(nil)), nil
}

// javySHA256 pins the official SHA-256 of each javy v5.0.1 `.gz` asset (from the
// release's `<asset>.gz.sha256`), keyed by archPlatform token. Bump with javyVersion.
var javySHA256 = map[string]string{
	"arm-macos":      "3169376ba098c90a16ccf4ccfeb268128b9eb1fd8fffb7a69eef4dc6376f7593",
	"x86_64-macos":   "ffd7d1feea29ad843f50a2840d9ff4ae2bb5cf76633bc2f4bccc0743b3a97718",
	"x86_64-linux":   "75d3da39560858f11dd8b7b923e6664fc63fa7cad4503510ca65951cd2e65531",
	"arm-linux":      "8bdb9eec05792eca33da5f1323b16467825d004e25234d7d7aa48ebe479b70e7",
	"x86_64-windows": "51c722b397576d3c1c3f5b5613d2cf6163fa301a8e7130a51043fbcd431f2add",
}

// Spec returns the binmgr.Spec for the pinned javy v5.0.1 release. URL maps
// GOOS/GOARCH to the release asset's archPlatform token; SHA256 returns the
// matching pinned checksum, failing closed for any platform without one.
func Spec() binmgr.Spec {
	return binmgr.Spec{
		Name:        javyName,
		Version:     javyVersion,
		Compression: "gzip",
		URL: func(goos, goarch string) (string, error) {
			archPlatform, err := resolveArchPlatform(goos, goarch)
			if err != nil {
				return "", err
			}
			url := fmt.Sprintf(
				"https://github.com/bytecodealliance/javy/releases/download/%s/javy-%s-%s.gz",
				javyVersion, archPlatform, javyVersion,
			)
			return url, nil
		},
		SHA256: func(goos, goarch string) (string, error) {
			archPlatform, err := resolveArchPlatform(goos, goarch)
			if err != nil {
				return "", err
			}
			sum, ok := javySHA256[archPlatform]
			if !ok {
				return "", fmt.Errorf("no pinned javy checksum for %s (%s/%s)", archPlatform, goos, goarch)
			}
			return sum, nil
		},
	}
}

// resolveArchPlatform maps a (goos, goarch) pair to the javy release archPlatform token.
func resolveArchPlatform(goos, goarch string) (string, error) {
	switch goos + "/" + goarch {
	case "darwin/arm64":
		return "arm-macos", nil
	case "darwin/amd64":
		return "x86_64-macos", nil
	case "linux/amd64":
		return "x86_64-linux", nil
	case "linux/arm64":
		return "arm-linux", nil
	case "windows/amd64":
		return "x86_64-windows", nil
	default:
		return "", fmt.Errorf("unsupported platform/architecture: %s/%s", goos, goarch)
	}
}

// Ensure returns the path to the cached javy binary, downloading and caching
// it under the user cache dir if not already present. A download failure
// surfaces binmgr's network/internal ExitError directly.
func Ensure(ctx context.Context) (string, *output.ExitError) {
	return binmgr.Ensure(ctx, Spec())
}
