package docs

import (
	"context"
	"errors"
	"fmt"
	"mime"
	"net/http"
	"net/url"
	"strings"
)

type Page struct {
	URL      string // after redirects, without .md
	Title    string // first H1
	Locale   string // zh-CN under /zh-CN/, else en
	Markdown string
}

// NormalizePageURL turns a www.shoplazza.dev URL or a site path into the page
// URL, without fragment, query, trailing slash or .md.
func NormalizePageURL(ref string) (string, error) {
	ref = strings.TrimSpace(ref)
	if ref == "" {
		return "", errors.New("--url is empty")
	}
	u, err := url.Parse(ref)
	if err != nil {
		return "", fmt.Errorf("invalid --url %q: %v", ref, err)
	}
	switch {
	case u.Scheme == "" && u.Host == "" && strings.HasPrefix(ref, "/"):
	case (u.Scheme == "https" || u.Scheme == "http") && u.Host != "":
		if !strings.EqualFold(u.Hostname(), SiteHost) {
			return "", fmt.Errorf("--url %q is not on %s", ref, SiteHost)
		}
	default:
		return "", fmt.Errorf("--url %q is neither a %s URL nor a site path starting with /", ref, SiteURL)
	}

	path := strings.TrimRight(u.EscapedPath(), "/")
	path = strings.TrimRight(strings.TrimSuffix(path, ".md"), "/")
	if path == "" {
		return "", fmt.Errorf("--url %q names the site root, not a page", ref)
	}
	return SiteURL + path, nil
}

// GetPage reads pageURL's .md copy, following redirects only on SiteHost.
func (c *Client) GetPage(ctx context.Context, pageURL string) (*Page, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, pageURL+".md", nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "text/markdown, text/plain;q=0.9, */*;q=0.8")
	req.Header.Set("User-Agent", c.UserAgent)

	hc := *c.HTTP
	hc.CheckRedirect = stayOnSite
	resp, err := hc.Do(req)
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
	// HTML means the page has no .md copy.
	if mt, _, _ := mime.ParseMediaType(resp.Header.Get("Content-Type")); mt == "text/html" || mt == "application/xhtml+xml" {
		return nil, ErrNotMarkdown
	}

	final := *resp.Request.URL
	final.RawQuery, final.Fragment = "", ""
	md := string(body)
	return &Page{
		URL:      strings.TrimSuffix(final.String(), ".md"),
		Title:    Title(md),
		Locale:   LocaleOf(final.Path),
		Markdown: md,
	}, nil
}

func stayOnSite(req *http.Request, via []*http.Request) error {
	if len(via) >= 10 {
		return ErrTooManyRedirects
	}
	if !strings.EqualFold(req.URL.Hostname(), SiteHost) {
		return fmt.Errorf("%w: %s", ErrOffSite, req.URL.Redacted())
	}
	return nil
}

func LocaleOf(path string) string {
	if path == "/zh-CN" || strings.HasPrefix(path, "/zh-CN/") {
		return "zh-CN"
	}
	return "en"
}

// Title returns the first "# " heading outside fenced code.
func Title(md string) string {
	fence := ""
	for _, line := range strings.Split(md, "\n") {
		line = strings.TrimRight(line, "\r")
		trimmed := strings.TrimLeft(line, " ")
		if len(line)-len(trimmed) > 3 {
			continue // indented code
		}
		if strings.HasPrefix(trimmed, "```") || strings.HasPrefix(trimmed, "~~~") {
			switch {
			case fence == "":
				fence = trimmed[:3]
			case strings.HasPrefix(trimmed, fence):
				fence = ""
			}
			continue
		}
		if fence != "" {
			continue
		}
		if trimmed != "#" && !strings.HasPrefix(trimmed, "# ") && !strings.HasPrefix(trimmed, "#\t") {
			continue
		}
		title := strings.TrimSpace(trimmed[1:])
		// A closing "#" sequence needs a space before it: "# C#" keeps its "#".
		if rest := strings.TrimRight(title, "#"); rest != title && (rest == "" || strings.HasSuffix(rest, " ") || strings.HasSuffix(rest, "\t")) {
			title = strings.TrimSpace(rest)
		}
		if title != "" {
			return title
		}
	}
	return ""
}

// Truncate cuts md to max characters and reports whether it cut.
func Truncate(md string, max int) (string, bool) {
	n := 0
	for i := range md {
		if n == max {
			return md[:i], true
		}
		n++
	}
	return md, false
}
