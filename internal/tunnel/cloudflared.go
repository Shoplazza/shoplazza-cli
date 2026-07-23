package tunnel

import (
	"bufio"
	"context"
	"fmt"
	"io"
	"os/exec"
	"regexp"
	"runtime"
	"time"

	"github.com/Shoplazza/shoplazza-cli/internal/binmgr"
	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// cloudflared pinned release.
const (
	cloudflaredVersion = "2024.8.2"
	cloudflaredBaseURL = "https://github.com/cloudflare/cloudflared/releases/download/" + cloudflaredVersion
)

// trycloudflareRe matches the quick-tunnel URL cloudflared prints on startup.
var trycloudflareRe = regexp.MustCompile(`https://[a-zA-Z0-9-]+\.trycloudflare\.com`)

// startupTimeout bounds how long we wait for cloudflared to print its URL
// before giving up.
const startupTimeout = 10 * time.Second

// Cloudflared is the primary tunnel strategy: it downloads/caches the pinned
// cloudflared binary and runs a quick tunnel (no account required).
type Cloudflared struct {
	// Progress, when non-nil, reports the download and tunnel-startup steps (live
	// elapsed timers) to the user. nil disables reporting.
	Progress *output.Progress
}

// Name implements Strategy.
func (*Cloudflared) Name() string { return "cloudflared" }

// cloudflaredSpec builds the binmgr.Spec for the given target platform.
// Compression is "tgz" on darwin (the .tgz assets contain a file named
// "cloudflared") and "" elsewhere (raw binaries).
func cloudflaredSpec(goos string) binmgr.Spec {
	compression := ""
	if goos == "darwin" {
		compression = "tgz"
	}
	return binmgr.Spec{
		Name:        "cloudflared",
		Version:     cloudflaredVersion,
		Compression: compression,
		URL:         cloudflaredURL,
		SHA256:      cloudflaredSHA256,
	}
}

// cloudflaredChecksums pins the SHA-256 of each cloudflared 2024.8.2 asset
// (the .tgz on darwin, the raw binary elsewhere), keyed by goos/goarch.
// cloudflare/cloudflared publishes no checksum file, so these were computed
// from the release assets with `shasum -a 256`; bump together with
// cloudflaredVersion.
var cloudflaredChecksums = map[string]string{
	"linux/amd64":   "e6cb78348e05680805c8317b5073c54401c1ebac9fa88a2cc35be752858bdc6b",
	"linux/arm64":   "f0cc2f42b658a89a794ca91210f73df2f3d51c459f050ae1ee57b221d1e30f98",
	"darwin/amd64":  "54c8988482cdc5ce187b1dbd3b6e055cce261079766a123d8584a4018d8d9cd2",
	"darwin/arm64":  "6e9888af320f356c71e45165f4627609dbfba0b772194d5019cbff22d90b41c9",
	"windows/amd64": "a054d767613ba64462dd457e3c0be27244c9484f4b7fcb76b37e137c86f0eda1",
}

// cloudflaredSHA256 returns the pinned checksum for a target platform, mirroring
// cloudflaredURL's platform set. Fail-closed: a platform without a pinned hash
// is rejected (so it can never silently exec unverified).
func cloudflaredSHA256(goos, goarch string) (string, error) {
	sum, ok := cloudflaredChecksums[goos+"/"+goarch]
	if !ok {
		return "", fmt.Errorf("no pinned cloudflared checksum for %s/%s", goos, goarch)
	}
	return sum, nil
}

// cloudflaredURL maps a target platform to its pinned cloudflared release asset.
// Unsupported platforms return an error.
func cloudflaredURL(goos, goarch string) (string, error) {
	var asset string
	switch goos {
	case "linux":
		switch goarch {
		case "amd64":
			asset = "cloudflared-linux-amd64"
		case "arm64":
			asset = "cloudflared-linux-arm64"
		}
	case "darwin":
		switch goarch {
		case "amd64":
			asset = "cloudflared-darwin-amd64.tgz"
		case "arm64":
			asset = "cloudflared-darwin-arm64.tgz"
		}
	case "windows":
		if goarch == "amd64" {
			asset = "cloudflared-windows-amd64.exe"
		}
	}
	if asset == "" {
		return "", fmt.Errorf("unsupported platform for cloudflared: %s/%s", goos, goarch)
	}
	return cloudflaredBaseURL + "/" + asset, nil
}

// Start implements Strategy: ensure the cloudflared binary, launch a quick
// tunnel to the local port, and return once the public URL is parsed from the
// process output. The 10s timeout bounds startup only — the returned Tunnel
// stays up under the caller's ctx until Close.
func (c *Cloudflared) Start(ctx context.Context, port int) (*Tunnel, error) {
	spec := cloudflaredSpec(runtime.GOOS)
	spec.Progress = c.Progress
	bin, xerr := binmgr.Ensure(ctx, spec)
	if xerr != nil {
		return nil, xerr
	}

	// Time the startup: from launch until cloudflared prints its public URL.
	step := c.Progress.Begin("Starting cloudflared tunnel")

	cmd := exec.CommandContext(ctx, bin, "tunnel", "--url",
		fmt.Sprintf("http://localhost:%d", port), "--no-autoupdate")

	// cloudflared prints the URL to stderr; merge both streams through one pipe
	// so a single scanner sees everything.
	pr, pw := io.Pipe()
	cmd.Stdout = pw
	cmd.Stderr = pw

	if err := cmd.Start(); err != nil {
		step.Fail()
		_ = pw.Close()
		return nil, fmt.Errorf("starting cloudflared: %w", err)
	}

	// Reap the process and unblock the scanner when it exits.
	exited := make(chan error, 1)
	go func() {
		exited <- cmd.Wait()
		_ = pw.Close()
	}()

	// Scan merged output for the tunnel URL; hand it off over a channel so the
	// goroutine never shares state with Start (clean under -race).
	urlCh := make(chan string, 1)
	go func() {
		sc := bufio.NewScanner(pr)
		for sc.Scan() {
			if u := parseTrycloudflareURL(sc.Text()); u != "" {
				urlCh <- u
				// Drain the rest so the process isn't blocked on a full pipe.
				_, _ = io.Copy(io.Discard, pr)
				return
			}
		}
	}()

	kill := func() error { return gracefulKill(cmd.Process) }

	select {
	case u := <-urlCh:
		step.Done()
		return &Tunnel{URL: u, Close: kill}, nil
	case err := <-exited:
		step.Fail()
		return nil, fmt.Errorf("cloudflared exited before reporting a tunnel URL: %v", err)
	case <-time.After(startupTimeout):
		step.Fail()
		_ = kill()
		return nil, fmt.Errorf("cloudflared did not report a tunnel URL within %s", startupTimeout)
	}
}

// parseTrycloudflareURL returns the trycloudflare.com URL embedded in line, or
// "" if none. Factored out for unit testing against sample cloudflared output.
func parseTrycloudflareURL(line string) string {
	return trycloudflareRe.FindString(line)
}
