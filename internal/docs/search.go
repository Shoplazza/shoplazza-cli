package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/url"
	"regexp"
	"strings"
)

// SearchRequest omits empty fields so the service applies its own defaults.
type SearchRequest struct {
	Query      string `json:"query"`
	Locale     string `json:"locale,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

var Locales = []string{"en", "zh-CN"}

var dashedVersion = regexp.MustCompile(`^\d{4}-\d{2}$`)

// NormalizeAPIVersion turns 2026-07 into 202607; the service validates the rest.
func NormalizeAPIVersion(v string) string {
	v = strings.TrimSpace(v)
	if dashedVersion.MatchString(v) {
		return strings.Replace(v, "-", "", 1)
	}
	return v
}

// Search returns the service's response with each result's url made absolute.
// "results" is always a list; scores stay json.Number.
func (c *Client) Search(ctx context.Context, req SearchRequest) (map[string]any, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
	}
	// Otherwise a bad SHOPLAZZA_DOCS_API surfaces as a network failure.
	if u, err := url.Parse(c.SearchAPI); err != nil || (u.Scheme != "http" && u.Scheme != "https") || u.Host == "" {
		return nil, fmt.Errorf("%w: %q", ErrSearchAPI, c.SearchAPI)
	}
	httpReq, err := http.NewRequestWithContext(ctx, http.MethodPost, c.SearchAPI+SearchPath, bytes.NewReader(payload))
	if err != nil {
		return nil, err
	}
	clientID := c.ClientID
	if clientID == "" {
		clientID = ClientID()
	}
	httpReq.Header.Set("Content-Type", "application/json")
	httpReq.Header.Set("Accept", "application/json")
	httpReq.Header.Set("User-Agent", c.UserAgent)
	httpReq.Header.Set("X-Client-Id", clientID)
	httpReq.Header.Set("X-Client-Surface", surface)

	resp, err := c.HTTP.Do(httpReq)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := readBody(resp)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, newHTTPError(resp, body)
	}

	var out map[string]any
	dec := json.NewDecoder(bytes.NewReader(body))
	dec.UseNumber()
	if err := dec.Decode(&out); err != nil || out == nil {
		return nil, fmt.Errorf("%w: the search response is not a JSON object", ErrBadResponse)
	}
	results, _ := out["results"].([]any)
	if results == nil {
		results = []any{}
	}
	for _, r := range results {
		if m, ok := r.(map[string]any); ok {
			if u, ok := m["url"].(string); ok {
				m["url"] = AbsoluteURL(u)
			}
		}
	}
	out["results"] = results
	return out, nil
}

// AbsoluteURL leaves anything but a site path as is.
func AbsoluteURL(path string) string {
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return SiteURL + path
	}
	return path
}
