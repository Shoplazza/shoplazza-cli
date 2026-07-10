package client

import (
	"fmt"
	"io"
	"net/http"
	"net/url"
	"path"
	"strings"
	"time"
)

// Client wraps the underlying HTTP client and target base URL.
type Client struct {
	BaseURL    string
	HTTPClient *http.Client
	Headers    map[string]string
	// Debug, when non-nil, dumps each request (method/URL/headers, auth values
	// redacted) and the raw response to it. Off by default.
	Debug io.Writer
}

// redactToken masks a secret header value for debug dumps, keeping just enough
// to correlate (first 6 + last 4) without leaking the token.
func redactToken(s string) string {
	if len(s) <= 12 {
		return "***"
	}
	return s[:6] + "…" + s[len(s)-4:]
}

// HTTPError wraps a non-2xx HTTP response. Method and Path name the failing
// request so a server error can identify which endpoint it came from.
type HTTPError struct {
	StatusCode int
	Body       string
	Method     string
	Path       string
}

func (e *HTTPError) Error() string {
	if e.Method != "" || e.Path != "" {
		return fmt.Sprintf("http request failed: %s %s status=%d body=%s", e.Method, e.Path, e.StatusCode, e.Body)
	}
	return fmt.Sprintf("http request failed: status=%d body=%s", e.StatusCode, e.Body)
}

// RequestSummary is a lightweight raw request preview.
type RequestSummary struct {
	Method string `json:"method"`
	Path   string `json:"path"`
	URL    string `json:"url"`
	Params any    `json:"params,omitempty"`
	Data   any    `json:"data,omitempty"`
}

// RawRequest is a generic HTTP request for the raw api layer.
type RawRequest struct {
	Method  string
	Path    string
	Params  map[string]any
	Data    any               // For DoRaw's multipart branch (Headers["Content-Type"] set), Data must be io.Reader, []byte, or untyped nil; typed-nil readers panic and strings must be wrapped as []byte or strings.NewReader.
	Headers map[string]string // Optional. Honored by DoRaw only; ignored by SendStream.
	// NoTimeout makes DoRaw bypass the client-wide HTTPClient.Timeout and rely
	// on ctx for cancellation instead. Set it for long-running transfers that
	// exceed the request timeout. Ignored by SendStream.
	NoTimeout bool
}

// RawResponse is the generic raw api execution result.
type RawResponse struct {
	StatusCode  int                 `json:"status_code"`
	ContentType string              `json:"content_type,omitempty"`
	Headers     map[string][]string `json:"headers,omitempty"`
	Body        any                 `json:"body,omitempty"`
}

// RequestID returns the first Request-Id header value, or "" if absent.
func (r RawResponse) RequestID() string {
	if v := r.Headers["Request-Id"]; len(v) > 0 {
		return v[0]
	}
	return ""
}

// New creates a minimal API client.
func New(baseURL string) *Client {
	return &Client{
		BaseURL: strings.TrimRight(baseURL, "/"),
		HTTPClient: &http.Client{
			Timeout: 30 * time.Second,
		},
		Headers: map[string]string{},
	}
}

// SetBaseURL updates the target base URL (e.g. once the auth gate resolves
// the store domain). Trailing slashes are stripped, matching New.
func (c *Client) SetBaseURL(u string) {
	c.BaseURL = strings.TrimRight(u, "/")
}

// ResolveURL resolves a request path against the configured base URL.
func (c *Client) ResolveURL(rawPath string) string {
	cleanPath := "/" + strings.TrimLeft(rawPath, "/")
	if c.BaseURL == "" {
		return cleanPath
	}
	return c.BaseURL + path.Clean(cleanPath)
}

// BuildRequestSummary creates a lightweight request preview.
func (c *Client) BuildRequestSummary(method, rawPath string, params, data any) RequestSummary {
	return RequestSummary{
		Method: method,
		Path:   rawPath,
		URL:    c.ResolveURL(rawPath),
		Params: params,
		Data:   data,
	}
}

// SetBearerToken sets the access token. Shoplazza OpenAPI uses Access-Token
// header (not Authorization: Bearer).
func (c *Client) SetBearerToken(token string) {
	if strings.TrimSpace(token) == "" {
		return
	}
	c.Headers["Access-Token"] = token
}

func encodeQuery(query map[string]any) url.Values {
	values := url.Values{}
	for key, raw := range query {
		if raw == nil {
			continue
		}
		switch v := raw.(type) {
		case string:
			if v != "" {
				values.Add(key, v)
			}
		case []string:
			for _, item := range v {
				if item != "" {
					values.Add(key, item)
				}
			}
		case []any:
			for _, item := range v {
				if item != nil {
					values.Add(key, fmt.Sprint(item))
				}
			}
		default:
			values.Add(key, fmt.Sprint(v))
		}
	}
	return values
}
