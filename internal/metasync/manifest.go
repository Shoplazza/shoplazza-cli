// Package metasync downloads the remote OpenAPI metadata spec into the local
// cache read by registry.LoadSpec, so metadata updates ship without a binary
// release. Every failure is non-fatal: the CLI keeps whatever spec it has.
package metasync

import (
	"bytes"
	"compress/gzip"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"os"
	"regexp"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/build"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

const (
	manifestPath = "manifest" // answers about the manifest, does not serve it
	specDir      = "specs/"
	// Per-payload budgets: the probe must not stall a command, the download is
	// up to 8 MB and has to be allowed to finish on a slow link.
	probeTimeout    = 5 * time.Second
	specTimeout     = 60 * time.Second
	maxManifestBody = 256 << 10 // 256 KB
	maxSpecBody     = 8 << 20   // 8 MB compressed download
	maxSpecRaw      = 32 << 20  // 32 MB decompressed
)

// metaRoutePrefix is the CliMetaService route; it shares the saiga auth host.
const metaRoutePrefix = "/api/saiga/cli/meta/"

// metaClient carries no client-wide timeout on purpose: the poll and the
// download are not the same wait, so each request's context holds the budget.
// Tests redirect it with EnvOrigin rather than swapping it out.
var metaClient = &http.Client{}

// manifest is the small remote index the client polls. UpToDate marks the
// server's "you already have this" answer, which carries no other field.
type manifest struct {
	Revision string `json:"revision"`
	Filename string `json:"filename"`
	SHA256   string `json:"sha256"`
	UpToDate bool   `json:"up_to_date,omitempty"`
}

// specNameRe is the only shape a spec archive name may take: a canonical UTC
// revision with colons stripped. An allowlist, so a server that named anything
// else — a path, another host — cannot steer the download.
var specNameRe = regexp.MustCompile(`^\d{4}-\d{2}-\d{2}T\d{6}Z\.json\.gz$`)

// originURL returns the metadata origin: EnvOrigin, else derived from the auth
// base URL.
func originURL() string {
	v := os.Getenv(EnvOrigin)
	if v == "" {
		base := os.Getenv("SHOPLAZZA_CLI_AUTH_BASE_URL")
		if base == "" {
			base = build.DefaultAuthBaseURL
		}
		v = strings.TrimRight(base, "/") + metaRoutePrefix
	}
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	return v
}

// getLimited GETs url within budget and returns at most limit bytes, erroring
// on overflow. The budget covers reading the body, not just the response head.
func getLimited(ctx context.Context, url string, limit int64, budget time.Duration) ([]byte, error) {
	ctx, cancel := context.WithTimeout(ctx, budget)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := metaClient.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("metasync: GET %s: HTTP %d", url, resp.StatusCode)
	}
	body, err := io.ReadAll(io.LimitReader(resp.Body, limit+1))
	if err != nil {
		return nil, err
	}
	if int64(len(body)) > limit {
		return nil, fmt.Errorf("metasync: GET %s: body exceeds %d bytes", url, limit)
	}
	return body, nil
}

// manifestURL reports what this build already has, for the server to decide on.
// cli_version rides along unread, so a per-version rollout stays server-side.
func manifestURL(cliVersion, localRevision string) string {
	q := url.Values{"cli_version": {cliVersion}, "revision": {localRevision}}
	return originURL() + manifestPath + "?" + q.Encode()
}

func fetchManifest(ctx context.Context, cliVersion, localRevision string) (*manifest, error) {
	body, err := getLimited(ctx, manifestURL(cliVersion, localRevision), maxManifestBody, probeTimeout)
	if err != nil {
		return nil, err
	}
	var m manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("metasync: manifest: %w", err)
	}
	// The up-to-date answer legitimately carries none of the fields below.
	if m.UpToDate {
		return &m, nil
	}
	if m.SHA256 == "" {
		return nil, errors.New("metasync: manifest missing sha256")
	}
	if !registry.IsCanonicalRevision(m.Revision) {
		return nil, fmt.Errorf("metasync: non-canonical manifest revision %q", m.Revision)
	}
	if !specNameRe.MatchString(m.Filename) {
		return nil, fmt.Errorf("metasync: invalid spec filename %q", m.Filename)
	}
	return &m, nil
}

// fetchSpec downloads the gzipped spec named by m, verifies its sha256 and
// returns the decompressed bytes.
func fetchSpec(ctx context.Context, m *manifest) ([]byte, error) {
	body, err := getLimited(ctx, originURL()+specDir+m.Filename, maxSpecBody, specTimeout)
	if err != nil {
		return nil, err
	}
	sum := sha256.Sum256(body)
	if !strings.EqualFold(hex.EncodeToString(sum[:]), m.SHA256) {
		return nil, errors.New("metasync: spec sha256 mismatch")
	}
	zr, err := gzip.NewReader(bytes.NewReader(body))
	if err != nil {
		return nil, fmt.Errorf("metasync: spec gzip: %w", err)
	}
	defer zr.Close()
	raw, err := io.ReadAll(io.LimitReader(zr, maxSpecRaw+1))
	if err != nil {
		return nil, fmt.Errorf("metasync: spec gunzip: %w", err)
	}
	if int64(len(raw)) > maxSpecRaw {
		return nil, fmt.Errorf("metasync: spec exceeds %d bytes decompressed", maxSpecRaw)
	}
	return raw, nil
}
