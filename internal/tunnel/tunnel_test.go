package tunnel

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
)

// fakeStrategy is an in-test Strategy: it either returns tunnel or fails with
// err, and records whether Start was called.
type fakeStrategy struct {
	name    string
	tunnel  *Tunnel
	err     error
	started bool
}

func (f *fakeStrategy) Name() string { return f.name }

func (f *fakeStrategy) Start(_ context.Context, _ int) (*Tunnel, error) {
	f.started = true
	return f.tunnel, f.err
}

func TestOpen_FirstStrategyWins(t *testing.T) {
	first := &fakeStrategy{name: "first", tunnel: &Tunnel{URL: "https://first.example"}}
	second := &fakeStrategy{name: "second", tunnel: &Tunnel{URL: "https://second.example"}}

	tun, xerr := Open(context.Background(), 3000, first, second)
	if xerr != nil {
		t.Fatalf("Open returned error: %v", xerr)
	}
	if tun.URL != "https://first.example" {
		t.Fatalf("got URL %q, want first strategy's URL", tun.URL)
	}
	if !first.started {
		t.Fatal("first strategy was not started")
	}
	if second.started {
		t.Fatal("second strategy was started but first succeeded")
	}
}

func TestOpen_FallbackOnError(t *testing.T) {
	first := &fakeStrategy{name: "first", err: errors.New("boom")}
	second := &fakeStrategy{name: "second", tunnel: &Tunnel{URL: "https://second.example"}}

	tun, xerr := Open(context.Background(), 3000, first, second)
	if xerr != nil {
		t.Fatalf("Open returned error: %v", xerr)
	}
	if tun.URL != "https://second.example" {
		t.Fatalf("got URL %q, want second strategy's URL", tun.URL)
	}
	if !first.started || !second.started {
		t.Fatalf("expected both strategies tried; first=%v second=%v", first.started, second.started)
	}
}

func TestOpen_AllFail(t *testing.T) {
	first := &fakeStrategy{name: "first", err: errors.New("boom-1")}
	second := &fakeStrategy{name: "second", err: errors.New("boom-2")}

	tun, xerr := Open(context.Background(), 3000, first, second)
	if tun != nil {
		t.Fatalf("expected nil tunnel, got %+v", tun)
	}
	if xerr == nil {
		t.Fatal("expected error when all strategies fail, got nil")
	}
	if xerr.Code != output.ExitNetwork {
		t.Fatalf("expected network exit code %d, got %d", output.ExitNetwork, xerr.Code)
	}
	if xerr.Detail == nil || xerr.Detail.Type != output.TypeNetwork {
		t.Fatalf("expected network-type error detail, got: %+v", xerr.Detail)
	}
	// Both strategy errors should be surfaced in the joined message.
	msg := xerr.Error()
	for _, want := range []string{"boom-1", "boom-2", "first", "second"} {
		if !strings.Contains(msg, want) {
			t.Fatalf("joined error %q missing %q", msg, want)
		}
	}
}

func TestParseTrycloudflareURL(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "real-looking cloudflared banner line",
			line: "2024-08-20T10:00:00Z INF +-----+ |  https://foo-bar.trycloudflare.com  | +-----+",
			want: "https://foo-bar.trycloudflare.com",
		},
		{
			name: "inline url",
			line: "Your quick Tunnel has been created! Visit it at: https://abc123-xyz.trycloudflare.com",
			want: "https://abc123-xyz.trycloudflare.com",
		},
		{
			name: "no match",
			line: "2024-08-20T10:00:00Z INF Requesting new quick Tunnel on trycloudflare.com...",
			want: "",
		},
		{
			name: "empty line",
			line: "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseTrycloudflareURL(tc.line); got != tc.want {
				t.Fatalf("parseTrycloudflareURL(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestParseNgrokURL(t *testing.T) {
	cases := []struct {
		name string
		line string
		want string
	}{
		{
			name: "started tunnel json line",
			line: `{"lvl":"info","msg":"started tunnel","obj":"tunnels","name":"command_line","addr":"http://localhost:3000","url":"https://abcd-1-2-3-4.ngrok-free.app"}`,
			want: "https://abcd-1-2-3-4.ngrok-free.app",
		},
		{
			name: "unrelated json line",
			line: `{"lvl":"info","msg":"client session established","obj":"tunnels.session"}`,
			want: "",
		},
		{
			name: "non-json line",
			line: "t=2024-08-20T10:00:00+0000 lvl=info msg=\"started tunnel\"",
			want: "",
		},
		{
			name: "empty line",
			line: "",
			want: "",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if got := parseNgrokURL(tc.line); got != tc.want {
				t.Fatalf("parseNgrokURL(%q) = %q, want %q", tc.line, got, tc.want)
			}
		})
	}
}

func TestCloudflaredURL(t *testing.T) {
	cases := []struct {
		goos, goarch string
		want         string
		wantErr      bool
	}{
		{"darwin", "arm64", cloudflaredBaseURL + "/cloudflared-darwin-arm64.tgz", false},
		{"darwin", "amd64", cloudflaredBaseURL + "/cloudflared-darwin-amd64.tgz", false},
		{"linux", "amd64", cloudflaredBaseURL + "/cloudflared-linux-amd64", false},
		{"linux", "arm64", cloudflaredBaseURL + "/cloudflared-linux-arm64", false},
		{"windows", "amd64", cloudflaredBaseURL + "/cloudflared-windows-amd64.exe", false},
		{"plan9", "mips", "", true},
		{"linux", "riscv64", "", true},
	}
	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			got, err := cloudflaredURL(tc.goos, tc.goarch)
			if tc.wantErr {
				if err == nil {
					t.Fatalf("expected error for %s/%s, got url %q", tc.goos, tc.goarch, got)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if got != tc.want {
				t.Fatalf("cloudflaredURL(%s,%s) = %q, want %q", tc.goos, tc.goarch, got, tc.want)
			}
		})
	}
}

func TestCloudflaredSpec_CompressionByOS(t *testing.T) {
	if s := cloudflaredSpec("darwin"); s.Compression != "tgz" {
		t.Fatalf("darwin compression = %q, want tgz", s.Compression)
	}
	if s := cloudflaredSpec("linux"); s.Compression != "" {
		t.Fatalf("linux compression = %q, want empty", s.Compression)
	}
	if s := cloudflaredSpec("windows"); s.Compression != "" {
		t.Fatalf("windows compression = %q, want empty", s.Compression)
	}
	s := cloudflaredSpec("darwin")
	if s.Name != "cloudflared" || s.Version != cloudflaredVersion {
		t.Fatalf("spec name/version = %q/%q, want cloudflared/%s", s.Name, s.Version, cloudflaredVersion)
	}
}

// TestCloudflaredSHA256_CoversEveryURLPlatform is the drift guard: every platform
// cloudflaredURL supports MUST have a pinned, non-empty checksum (so a future
// platform addition can't silently exec an unverified binary), and unsupported
// platforms must be rejected symmetrically.
func TestCloudflaredSHA256_CoversEveryURLPlatform(t *testing.T) {
	cases := []struct{ goos, goarch string }{
		{"darwin", "arm64"}, {"darwin", "amd64"},
		{"linux", "amd64"}, {"linux", "arm64"},
		{"windows", "amd64"},
		{"plan9", "mips"}, {"linux", "riscv64"}, // unsupported
	}
	for _, tc := range cases {
		t.Run(tc.goos+"/"+tc.goarch, func(t *testing.T) {
			_, urlErr := cloudflaredURL(tc.goos, tc.goarch)
			sum, shaErr := cloudflaredSpec(tc.goos).SHA256(tc.goos, tc.goarch)
			if urlErr == nil {
				if shaErr != nil {
					t.Fatalf("URL supports %s/%s but SHA256 errored: %v", tc.goos, tc.goarch, shaErr)
				}
				if sum == "" {
					t.Fatalf("URL supports %s/%s but SHA256 is empty (silent-skip risk)", tc.goos, tc.goarch)
				}
			} else if shaErr == nil {
				t.Fatalf("URL rejects %s/%s but SHA256 returned %q (inconsistent)", tc.goos, tc.goarch, sum)
			}
		})
	}
}
