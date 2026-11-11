// Package docs searches and reads the developer docs on www.shoplazza.dev.
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
	EnvSearchAPI     = "SHOPLAZZA_DOCS_API"
	DefaultSearchAPI = "https://eco-agent.shoplazza.com"
	SearchPath       = "/api/docs/search"

	SiteHost = "www.shoplazza.dev"
	SiteURL  = "https://" + SiteHost

	surface      = "cli"
	maxBodyBytes = 32 << 20
)

type Client struct {
	SearchAPI string // no trailing slash
	HTTP      *http.Client
	ClientID  string // X-Client-Id; empty means ClientID()
	UserAgent string
}

// NewClient honors SHOPLAZZA_DOCS_API.
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

type HTTPError struct {
	StatusCode int
	Method     string
	Path       string // after redirects, without query
	Body       string
	TraceID    string // X-Trace-Id, else the body's trace_id
	RetryAfter string
}

func (e *HTTPError) Error() string {
	return fmt.Sprintf("%s %s: status %d", e.Method, e.Path, e.StatusCode)
}

var (
	ErrOffSite          = errors.New("redirected off " + SiteHost)
	ErrTooManyRedirects = errors.New("stopped after 10 redirects")
	ErrSearchAPI        = errors.New("the search service URL is not an http(s) URL")
	// ErrBadResponse is a 2xx response the CLI can't use.
	ErrBadResponse = errors.New("unexpected response")
	ErrNotMarkdown = fmt.Errorf("%w: the site returned HTML, not Markdown", ErrBadResponse)
)

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

func readBody(resp *http.Response) ([]byte, error) {
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxBodyBytes+1))
	if err != nil {
		return nil, err
	}
	if len(data) > maxBodyBytes {
		return nil, fmt.Errorf("%w: %s %s is larger than %d MiB", ErrBadResponse, resp.Request.Method, resp.Request.URL.Redacted(), maxBodyBytes>>20)
	}
	return data, nil
}
