// Package docs searches and reads the Shoplazza developer docs on
// www.shoplazza.dev, for `shoplazza docs`. Neither needs a login.
package docs

import (
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

const (
	// EnvSearchAPI overrides the search service URL.
	EnvSearchAPI = "SHOPLAZZA_DOCS_API"
	// DefaultSearchAPI is the search service URL.
	DefaultSearchAPI = "https://eco-agent.shoplazza.com"
	// SearchPath is the search endpoint.
	SearchPath = "/api/docs/search"

	// SiteHost is the docs site; pages are read from it only.
	SiteHost = "www.shoplazza.dev"
	SiteURL  = "https://" + SiteHost

	surface      = "cli"
	maxBodyBytes = 32 << 20
)

// Client calls the search service and the docs site.
type Client struct {
	SearchAPI string // search service URL, no trailing slash
	HTTP      *http.Client
	ClientID  string // X-Client-Id; empty means ClientID()
	UserAgent string
}

// NewClient returns a client for CLI version, honoring SHOPLAZZA_DOCS_API.
func NewClient(version string) *Client {
	api := strings.TrimRight(strings.TrimSpace(os.Getenv(EnvSearchAPI)), "/")
	if api == "" {
		api = DefaultSearchAPI
	}
	return &Client{
		SearchAPI: api,
		HTTP:      &http.Client{Timeout: client.DefaultTimeout},
		UserAgent: "shoplazza-cli/" + strings.TrimPrefix(version, "v"),
	}
}

// HTTPError is a non-2xx response.
type HTTPError struct {
	StatusCode int
	Method     string
	Path       string // request path after redirects, without query
	Body       string
	TraceID    string // X-Trace-Id, else the body's trace_id
	RetryAfter string // Retry-After header
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d", e.Method, e.Path, e.StatusCode)
}

// ErrOffSite reports a redirect that leaves SiteHost.
var ErrOffSite = errors.New("redirected off " + SiteHost)

func newHTTPError(resp *http.Response, body []byte) *HTTPError {
	return &HTTPError{
		StatusCode: resp.StatusCode,
		Method:     resp.Request.Method,
		Path:       resp.Request.URL.EscapedPath(),
		Body:       string(body),
		TraceID:    traceID(resp.Header, body),
		RetryAfter: resp.Header.Get("Retry-After"),
	}
}

func traceID(h http.Header, body []byte) string {
	if id := h.Get("X-Trace-Id"); id != "" {
		return id
	}
	var b struct {
		TraceID string `json:"trace_id"`
	}
	_ = json.Unmarshal(body, &b)
	return b.TraceID
}

// readBody reads at most maxBodyBytes of the response body.
func readBody(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("%s %s: response is larger than %d MiB", resp.Request.Method, resp.Request.URL.Redacted(), maxBodyBytes>>20)
	}
	return data, nil
}
