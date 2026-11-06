package docs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"regexp"
	"strings"
)

// SearchRequest is the search body. Empty fields are omitted, so the service
// applies its own defaults and a new default needs no CLI release.
type SearchRequest struct {
	Query      string `json:"query"`
	Locale     string `json:"locale,omitempty"`
	APIVersion string `json:"api_version,omitempty"`
	Limit      int    `json:"limit,omitempty"`
}

// Locales are the docs languages.
var Locales = []string{"en", "zh-CN"}

var dashedVersion = regexp.MustCompile(`^\d{4}-\d{2}$`)

// NormalizeAPIVersion turns 2026-07 into 202607. Other values pass through
// trimmed; the service validates them.
func NormalizeAPIVersion(v string) string {
	v = strings.TrimSpace(v)
	if dashedVersion.MatchString(v) {
		return strings.Replace(v, "-", "", 1)
	}
	return v
}

// Search returns the service's response with each result's url made absolute.
// Each result is one passage of a page, so a page may appear more than once.
// "results" is always a list; scores stay json.Number.
func (c *Client) Search(ctx context.Context, req SearchRequest) (map[string]any, error) {
	payload, err := json.Marshal(req)
	if err != nil {
		return nil, err
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
	if err := dec.Decode(&out); err != nil {
		return nil, fmt.Errorf("decode search response: %w", err)
	}
	if out == nil {
		return nil, errors.New("decode search response: not a JSON object")
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

// AbsoluteURL joins a site path to SiteURL; other values are returned as is.
func AbsoluteURL(path string) string {
	if strings.HasPrefix(path, "/") && !strings.HasPrefix(path, "//") {
		return SiteURL + path
	}
	return path
}
