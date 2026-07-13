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
	"os"
	"strings"
	"time"
)

const (
	manifestName    = "manifest.json"
	formatVersion   = 1
	fetchTimeout    = 5 * time.Second
	maxManifestBody = 256 << 10 // 256 KB
	maxSpecBody     = 8 << 20   // 8 MB compressed download
	maxSpecRaw      = 32 << 20  // 32 MB decompressed
)

// defaultOrigin is the static hosting root for manifest.json and specs/.
// Placeholder until infra provisions the bucket; overridable per-run via
// SHOPLAZZA_CLI_META_ORIGIN.
var defaultOrigin = "https://static.shoplazza.com/shoplazza-cli/meta/"

// DefaultClient overrides the HTTP client (for tests). nil -> default client with timeout.
var DefaultClient *http.Client

// Manifest is the small remote index the client polls.
type Manifest struct {
	FormatVersion int    `json:"format_version"`
	Revision      string `json:"revision"`
	MinCLIVersion string `json:"min_cli_version,omitempty"`
	URL           string `json:"url"`
	SHA256        string `json:"sha256"`
	Size          int64  `json:"size,omitempty"`
}

func originURL() string {
	v := os.Getenv("SHOPLAZZA_CLI_META_ORIGIN")
	if v == "" {
		v = defaultOrigin
	}
	if !strings.HasSuffix(v, "/") {
		v += "/"
	}
	return v
}

func httpClient() *http.Client {
	if DefaultClient != nil {
		return DefaultClient
	}
	return &http.Client{Timeout: fetchTimeout}
}

// getLimited GETs url and returns at most limit bytes, erroring on overflow.
func getLimited(ctx context.Context, url string, limit int64) ([]byte, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, url, nil)
	if err != nil {
		return nil, err
	}
	resp, err := httpClient().Do(req)
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

func fetchManifest(ctx context.Context) (*Manifest, error) {
	body, err := getLimited(ctx, originURL()+manifestName, maxManifestBody)
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err := json.Unmarshal(body, &m); err != nil {
		return nil, fmt.Errorf("metasync: manifest: %w", err)
	}
	if m.Revision == "" || m.URL == "" || m.SHA256 == "" {
		return nil, errors.New("metasync: manifest missing required fields")
	}
	return &m, nil
}

// fetchSpec downloads the gzipped spec named by m, verifies its sha256 and
// returns the decompressed bytes.
func fetchSpec(ctx context.Context, m *Manifest) ([]byte, error) {
	body, err := getLimited(ctx, originURL()+m.URL, maxSpecBody)
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
