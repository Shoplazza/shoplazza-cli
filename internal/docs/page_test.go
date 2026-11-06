package docs

import (
	"context"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func TestNormalizePageURL(t *testing.T) {
	ok := []struct{ in, want string }{
		{"/docs/app/x", SiteURL + "/docs/app/x"},
		{"/docs/app/x/", SiteURL + "/docs/app/x"},
		{"/docs/app/x.md", SiteURL + "/docs/app/x"},
		{"/docs/app/x/#oauth?a=1", SiteURL + "/docs/app/x"},
		{"/zh-CN/api/202601/product?x=1#y", SiteURL + "/zh-CN/api/202601/product"},
		{"http://WWW.shoplazza.dev/docs/app/x.md#a", SiteURL + "/docs/app/x"},
		{"  https://www.shoplazza.dev/api/product-create/  ", SiteURL + "/api/product-create"},
	}
	for _, c := range ok {
		got, err := NormalizePageURL(c.in)
		if err != nil || got != c.want {
			t.Errorf("NormalizePageURL(%q) = %q, %v; want %q", c.in, got, err, c.want)
		}
	}
	for _, in := range []string{
		"", "https://shoplazza.dev/docs/x", "https://evil.example.com/docs/x", "//evil.example.com/docs/x",
		"docs/app/x", "ftp://www.shoplazza.dev/docs/x", "https://www.shoplazza.dev/", "/.md",
	} {
		if got, err := NormalizePageURL(in); err == nil {
			t.Errorf("NormalizePageURL(%q) = %q, want an error", in, got)
		}
	}
}

func TestTitle(t *testing.T) {
	for md, want := range map[string]string{
		"# Security\n\ntext":                        "Security",
		"**Version: 202607**\n\n# Create product\n": "Create product",
		"```bash\n# not a title\n```\n# Real":       "Real",
		"~~~\n# no\n~~~\n## H2\n# Yes #\n":          "Yes",
		"# C#\n":                                    "C#",
		"    # indented code\n# T":                  "T",
		"#\n# Second\n":                             "Second",
		"## only h2\n#hashtag\n":                    "",
		"# 安全\r\n\r\n正文":                            "安全",
	} {
		if got := Title(md); got != want {
			t.Errorf("Title(%q) = %q, want %q", md, got, want)
		}
	}
}

func TestTruncate(t *testing.T) {
	cases := []struct {
		md   string
		max  int
		want string
		cut  bool
	}{
		{"abcde", 5, "abcde", false},
		{"line one\nline two\nline three", 15, "line one\n", true},
		{"中文\n中文中文", 4, "中文\n", true}, // characters, not bytes
		{"一二三四五六", 3, "一二三", true},    // no line break: cut at max
	}
	for _, c := range cases {
		if got, cut := Truncate(c.md, c.max); got != c.want || cut != c.cut {
			t.Errorf("Truncate(%q, %d) = %q, %v; want %q, %v", c.md, c.max, got, cut, c.want, c.cut)
		}
	}
}

func TestLocaleOf(t *testing.T) {
	for path, want := range map[string]string{
		"/zh-CN/docs/app/x": "zh-CN", "/zh-CN": "zh-CN", "/docs/app/x": "en", "/zh-CNx/docs": "en",
	} {
		if got := LocaleOf(path); got != want {
			t.Errorf("LocaleOf(%q) = %q, want %q", path, got, want)
		}
	}
}

func TestGetPage(t *testing.T) {
	var ua string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.URL.Path {
		case "/zh-CN/docs/old.md":
			http.Redirect(w, r, SiteURL+"/zh-CN/docs/new.md", http.StatusMovedPermanently)
		case "/zh-CN/docs/new.md":
			ua = r.Header.Get("User-Agent")
			_, _ = io.WriteString(w, "**版本：202607**\n\n# 新页面\n\n正文\n")
		case "/docs/away.md":
			http.Redirect(w, r, "https://evil.example.com/away.md", http.StatusFound)
		default:
			http.NotFound(w, r)
		}
	}))
	defer srv.Close()
	c := testClient(srv)

	page, err := c.GetPage(context.Background(), SiteURL+"/zh-CN/docs/old")
	if err != nil {
		t.Fatal(err)
	}
	if page.URL != SiteURL+"/zh-CN/docs/new" || page.Title != "新页面" || page.Locale != "zh-CN" || !strings.Contains(page.Markdown, "正文") {
		t.Errorf("page = %+v", page)
	}
	if ua != "shoplazza-cli/9.9.9" {
		t.Errorf("User-Agent = %q", ua)
	}

	if _, err := c.GetPage(context.Background(), SiteURL+"/docs/away"); !errors.Is(err, ErrOffSite) {
		t.Errorf("off-site redirect: err = %v, want ErrOffSite", err)
	}

	_, err = c.GetPage(context.Background(), SiteURL+"/docs/missing")
	var he *HTTPError
	if !errors.As(err, &he) || he.StatusCode != 404 || he.Path != "/docs/missing.md" {
		t.Errorf("missing page: err = %v, want a 404 for /docs/missing.md", err)
	}
}
