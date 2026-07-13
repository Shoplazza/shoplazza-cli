package metasync

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"sync/atomic"
	"testing"
	"time"

	"shoplazza-cli-v2/internal/registry"
	"shoplazza-cli-v2/internal/testenv"
)

// futureRev sorts after any real embedded generated_at; pastRev before.
const (
	futureRev = "9998-01-01T00:00:00Z"
	pastRev   = "1970-01-01T00:00:00Z"
)

func gzipBytes(t *testing.T, data []byte) []byte {
	t.Helper()
	var buf bytes.Buffer
	zw := gzip.NewWriter(&buf)
	if _, err := zw.Write(data); err != nil {
		t.Fatal(err)
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	return buf.Bytes()
}

func sha256Hex(data []byte) string {
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func specJSON(rev string) []byte {
	return []byte(`{"version":"vTEST","generated_at":"` + rev + `","modules":[{"name":"zz-sync-probe","commands":[]}]}`)
}

// remote is a fake origin serving one manifest + one gzipped spec.
type remote struct {
	srv      *httptest.Server
	manifest []byte
	spec     []byte // gzipped body served at specs/spec.json.gz
	specHits atomic.Int64
}

func newRemote(t *testing.T, manifest Manifest, gzSpec []byte) *remote {
	t.Helper()
	r := &remote{spec: gzSpec}
	mb, err := json.Marshal(manifest)
	if err != nil {
		t.Fatal(err)
	}
	r.manifest = mb
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		_, _ = w.Write(r.manifest)
	})
	mux.HandleFunc("/specs/spec.json.gz", func(w http.ResponseWriter, _ *http.Request) {
		r.specHits.Add(1)
		_, _ = w.Write(r.spec)
	})
	r.srv = httptest.NewServer(mux)
	t.Cleanup(r.srv.Close)
	return r
}

// setup isolates the config dir, points the origin at the fake remote, and
// clears every skip-guard env so tests behave the same locally and in CI.
func setup(t *testing.T, r *remote) {
	t.Helper()
	testenv.IsolateConfigDir(t)
	origin := ""
	if r != nil {
		origin = r.srv.URL + "/"
	}
	t.Setenv("SHOPLAZZA_CLI_META_ORIGIN", origin)
	for _, k := range []string{"CI", "BUILD_NUMBER", "RUN_ID", "SHOPLAZZA_CLI_NO_META_UPDATE"} {
		t.Setenv(k, "")
	}
}

func readCache(t *testing.T) []byte {
	t.Helper()
	path, err := registry.CachedSpecPath()
	if err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return nil
	}
	return data
}

func manifestFor(raw []byte, rev string) Manifest {
	return Manifest{FormatVersion: 1, Revision: rev, URL: "specs/spec.json.gz", SHA256: sha256Hex(raw)}
}

func TestDoRefresh_HappyPath(t *testing.T) {
	raw := specJSON(futureRev)
	gz := gzipBytes(t, raw)
	r := newRemote(t, manifestFor(gz, futureRev), gz)
	setup(t, r)

	res, err := ForceRefresh(context.Background(), "1.0.0")
	if err != nil {
		t.Fatal(err)
	}
	if !res.Updated || res.NewRevision != futureRev {
		t.Fatalf("unexpected result: %+v", res)
	}
	if !bytes.Equal(readCache(t), raw) {
		t.Fatal("cache file must hold the decompressed spec")
	}
	s := loadState()
	if s == nil || s.Revision != futureRev || s.LastCheckedAt == 0 {
		t.Fatalf("unexpected state: %+v", s)
	}
}

func TestDoRefresh_Gates(t *testing.T) {
	cases := []struct {
		name     string
		mutate   func(m *Manifest)
		version  string
		wantHits int64 // spec endpoint hits
	}{
		{name: "old revision skipped", mutate: func(m *Manifest) { m.Revision = pastRev }, version: "1.0.0"},
		{name: "min_cli_version too high skipped", mutate: func(m *Manifest) { m.MinCLIVersion = "999.0.0" }, version: "1.0.0"},
		{name: "unknown format_version skipped", mutate: func(m *Manifest) { m.FormatVersion = 2 }, version: "1.0.0"},
		{name: "dev build passes min gate", mutate: func(m *Manifest) { m.MinCLIVersion = "999.0.0" }, version: "dev", wantHits: 1},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := specJSON(futureRev)
			gz := gzipBytes(t, raw)
			m := manifestFor(gz, futureRev)
			tc.mutate(&m)
			r := newRemote(t, m, gz)
			setup(t, r)

			res, err := ForceRefresh(context.Background(), tc.version)
			if err != nil {
				t.Fatal(err)
			}
			if got := r.specHits.Load(); got != tc.wantHits {
				t.Fatalf("spec hits = %d, want %d", got, tc.wantHits)
			}
			if res.Updated != (tc.wantHits == 1) {
				t.Fatalf("Updated = %v, want %v", res.Updated, tc.wantHits == 1)
			}
			// A fully processed gate must advance the TTL clock.
			if s := loadState(); s == nil || s.LastCheckedAt == 0 {
				t.Fatalf("state must record the completed check, got %+v", s)
			}
		})
	}
}

func TestDoRefresh_RejectsBadPayloads(t *testing.T) {
	valid := specJSON(futureRev)
	cases := []struct {
		name     string
		manifest func(gz []byte) Manifest
		body     func(t *testing.T) []byte
	}{
		{
			name:     "sha256 mismatch",
			manifest: func(gz []byte) Manifest { m := manifestFor(gz, futureRev); m.SHA256 = sha256Hex([]byte("x")); return m },
			body:     func(t *testing.T) []byte { return gzipBytes(t, valid) },
		},
		{
			name:     "corrupt gzip",
			manifest: func(gz []byte) Manifest { return manifestFor(gz, futureRev) },
			body:     func(_ *testing.T) []byte { return []byte("not a gzip stream") },
		},
		{
			name:     "revision mismatch inside spec",
			manifest: func(gz []byte) Manifest { return manifestFor(gz, futureRev) },
			body:     func(t *testing.T) []byte { return gzipBytes(t, specJSON("9997-01-01T00:00:00Z")) },
		},
		{
			name:     "empty modules",
			manifest: func(gz []byte) Manifest { return manifestFor(gz, futureRev) },
			body: func(t *testing.T) []byte {
				return gzipBytes(t, []byte(`{"version":"v","generated_at":"`+futureRev+`","modules":[]}`))
			},
		},
		{
			name:     "spec not valid json",
			manifest: func(gz []byte) Manifest { return manifestFor(gz, futureRev) },
			body:     func(t *testing.T) []byte { return gzipBytes(t, []byte("{nope")) },
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			body := tc.body(t)
			r := newRemote(t, tc.manifest(body), body)
			setup(t, r)

			if _, err := ForceRefresh(context.Background(), "1.0.0"); err == nil {
				t.Fatal("expected error")
			}
			if readCache(t) != nil {
				t.Fatal("failed refresh must not write the cache file")
			}
			// Failures must NOT advance the TTL clock (retry next run).
			if s := loadState(); s != nil {
				t.Fatalf("failed refresh must not write state, got %+v", s)
			}
		})
	}
}

func TestDoRefresh_OversizedManifestRejected(t *testing.T) {
	raw := specJSON(futureRev)
	gz := gzipBytes(t, raw)
	r := newRemote(t, manifestFor(gz, futureRev), gz)
	r.manifest = bytes.Repeat([]byte("a"), maxManifestBody+1)
	setup(t, r)

	if _, err := ForceRefresh(context.Background(), "1.0.0"); err == nil {
		t.Fatal("expected error for oversized manifest")
	}
}

func TestDoRefresh_ManifestHTTPError(t *testing.T) {
	mux := http.NewServeMux()
	mux.HandleFunc("/manifest.json", func(w http.ResponseWriter, _ *http.Request) {
		http.Error(w, "nope", http.StatusNotFound)
	})
	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)
	testenv.IsolateConfigDir(t)
	t.Setenv("SHOPLAZZA_CLI_META_ORIGIN", srv.URL+"/")

	if _, err := ForceRefresh(context.Background(), "1.0.0"); err == nil {
		t.Fatal("expected error for HTTP 404 manifest")
	}
}

func TestDoRefresh_FailureKeepsExistingCache(t *testing.T) {
	raw := specJSON(futureRev)
	gz := gzipBytes(t, raw)
	r := newRemote(t, manifestFor(gz, futureRev), gz)
	setup(t, r)
	if _, err := ForceRefresh(context.Background(), "1.0.0"); err != nil {
		t.Fatal(err)
	}
	// Second refresh against a broken remote: cache must survive untouched.
	m := manifestFor(gz, "9999-01-01T00:00:00Z")
	m.SHA256 = sha256Hex([]byte("broken"))
	mb, _ := json.Marshal(m)
	r.manifest = mb
	if _, err := ForceRefresh(context.Background(), "1.0.0"); err == nil {
		t.Fatal("expected error")
	}
	if !bytes.Equal(readCache(t), raw) {
		t.Fatal("existing cache must survive a failed refresh")
	}
}

func TestRefresh_TTLGuard(t *testing.T) {
	raw := specJSON(futureRev)
	gz := gzipBytes(t, raw)
	r := newRemote(t, manifestFor(gz, futureRev), gz)
	setup(t, r)

	if err := saveState(&state{LastCheckedAt: time.Now().Unix()}); err != nil {
		t.Fatal(err)
	}
	Refresh(context.Background(), "1.0.0")
	if got := r.specHits.Load(); got != 0 {
		t.Fatalf("fresh TTL must skip the network, spec hits = %d", got)
	}
	if readCache(t) != nil {
		t.Fatal("TTL-guarded refresh must not write cache")
	}
}

func TestRefresh_SkipGuards(t *testing.T) {
	cases := []struct {
		name    string
		env     map[string]string
		version string
	}{
		{name: "disabled by env", env: map[string]string{"SHOPLAZZA_CLI_NO_META_UPDATE": "1"}, version: "1.0.0"},
		{name: "ci env", env: map[string]string{"CI": "true"}, version: "1.0.0"},
		{name: "dev version", env: nil, version: "dev"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			raw := specJSON(futureRev)
			gz := gzipBytes(t, raw)
			r := newRemote(t, manifestFor(gz, futureRev), gz)
			setup(t, r)
			for k, v := range tc.env {
				t.Setenv(k, v)
			}
			Refresh(context.Background(), tc.version)
			if got := r.specHits.Load(); got != 0 {
				t.Fatalf("skip guard must prevent download, spec hits = %d", got)
			}
		})
	}
}

func TestRefresh_EndToEnd(t *testing.T) {
	raw := specJSON(futureRev)
	gz := gzipBytes(t, raw)
	r := newRemote(t, manifestFor(gz, futureRev), gz)
	setup(t, r)

	Refresh(context.Background(), "1.0.0")
	if !bytes.Equal(readCache(t), raw) {
		t.Fatal("background refresh must write the cache")
	}
	// Second call inside the TTL window must be a no-op.
	Refresh(context.Background(), "1.0.0")
	if got := r.specHits.Load(); got != 1 {
		t.Fatalf("TTL must suppress the second download, spec hits = %d", got)
	}
}

func TestTooOld(t *testing.T) {
	cases := []struct {
		min, current string
		want         bool
	}{
		{"", "1.0.0", false},
		{"2.0.0", "1.0.0", true},
		{"2.0.0", "2.0.0", false},
		{"2.0.0", "3.0.0", false},
		{"2.0.0", "dev", false},     // non-release builds always pass
		{"garbage", "1.0.0", false}, // unparseable min never blocks
	}
	for _, tc := range cases {
		if got := tooOld(tc.min, tc.current); got != tc.want {
			t.Errorf("tooOld(%q, %q) = %v, want %v", tc.min, tc.current, got, tc.want)
		}
	}
}

func TestCurrentStatus(t *testing.T) {
	testenv.IsolateConfigDir(t)
	st := CurrentStatus()
	if st.Source == "" || st.Revision == "" {
		t.Fatalf("status must report source and revision, got %+v", st)
	}
	if !st.LastCheckedAt.IsZero() {
		t.Fatalf("no state file means zero LastCheckedAt, got %v", st.LastCheckedAt)
	}
	if err := saveState(&state{LastCheckedAt: 1700000000, Revision: "r"}); err != nil {
		t.Fatal(err)
	}
	if got := CurrentStatus().LastCheckedAt; got.IsZero() {
		t.Fatal("LastCheckedAt must reflect the state file")
	}
}

func TestOriginURL(t *testing.T) {
	t.Setenv("SHOPLAZZA_CLI_META_ORIGIN", "http://example.test/meta")
	if got := originURL(); got != "http://example.test/meta/" {
		t.Fatalf("originURL() = %q, want trailing slash added", got)
	}
	t.Setenv("SHOPLAZZA_CLI_META_ORIGIN", "")
	if got := originURL(); got != defaultOrigin {
		t.Fatalf("originURL() = %q, want default %q", got, defaultOrigin)
	}
}

func TestMain(m *testing.M) {
	// Belt and braces: never let a stray test hit the real default origin.
	defaultOrigin = fmt.Sprintf("http://127.0.0.1:0/unreachable-%d/", os.Getpid())
	os.Exit(m.Run())
}
