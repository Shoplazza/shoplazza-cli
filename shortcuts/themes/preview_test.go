package themes

import (
	"context"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// TestPreviewDerivesDomainFromClient asserts the store domain comes from the
// client base URL and that no API call is made.
func TestPreviewDerivesDomainFromClient(t *testing.T) {
	in := common.ExecInput{
		Flags:  shortcutFlags(t, previewShortcut, map[string]any{"theme-id": "abc"}),
		Client: client.New("https://demo.myshoplaza.com"),
	}

	res, err := previewShortcut.Execute(context.Background(), in)
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if len(res.Plans) != 0 {
		t.Errorf("+preview must make no API call, got plans: %+v", res.Plans)
	}
	if got := res.Body["store_domain"]; got != "demo.myshoplaza.com" {
		t.Errorf("store_domain = %v, want demo.myshoplaza.com", got)
	}
	if got := res.Body["preview_url"]; got != "https://demo.myshoplaza.com/?preview_theme_id=abc" {
		t.Errorf("preview_url = %v, want https://demo.myshoplaza.com/?preview_theme_id=abc", got)
	}
}

// TestBuildPreviewURL_NoSession covers the plain preview URL (no --oseid).
func TestBuildPreviewURL_NoSession(t *testing.T) {
	cases := []struct {
		name    string
		path    string
		themeID string
		locale  string
		want    string
	}{
		{
			name:    "default path, no locale",
			path:    "/",
			themeID: "abc",
			want:    "https://shop.myshoplaza.com/?preview_theme_id=abc",
		},
		{
			name:    "empty path defaults to root",
			path:    "",
			themeID: "abc",
			want:    "https://shop.myshoplaza.com/?preview_theme_id=abc",
		},
		{
			name:    "relative path gets leading slash",
			path:    "products/xyz",
			themeID: "abc",
			want:    "https://shop.myshoplaza.com/products/xyz?preview_theme_id=abc",
		},
		{
			name:    "locale appended when set",
			path:    "/",
			themeID: "abc",
			locale:  "zh_CN",
			want:    "https://shop.myshoplaza.com/?preview_theme_id=abc&locale=zh_CN",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := buildPreviewURL("shop.myshoplaza.com", tc.path, tc.themeID, "", tc.locale)
			if got != tc.want {
				t.Errorf("buildPreviewURL() = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBuildPreviewURL_Session covers the edit-session preview URL (--oseid set).
func TestBuildPreviewURL_Session(t *testing.T) {
	got := buildPreviewURL("shop.myshoplaza.com", "/", "abc", "sess-1", "")

	for _, want := range []string{
		"https://shop.myshoplaza.com/?",
		"&oseid=sess-1",
		"&preview_theme_id=abc",
		"&locale=en_US", // default when --locale omitted
		"&st=",
	} {
		if !strings.Contains(got, want) {
			t.Errorf("session URL missing %q in %q", want, got)
		}
	}

	// explicit locale wins over the en_US default
	got = buildPreviewURL("shop.myshoplaza.com", "/", "abc", "sess-1", "zh_CN")
	if !strings.Contains(got, "&locale=zh_CN") {
		t.Errorf("session URL should honour explicit locale, got %q", got)
	}
}

// TestBuildPreviewURL_PathWithQuery: a path that already carries a query
// (custom template's template=) is continued with & rather than a second ?.
func TestBuildPreviewURL_PathWithQuery(t *testing.T) {
	got := buildPreviewURL("shop.myshoplaza.com", "pages/about-us?template=abc", "t1", "", "")
	if got != "https://shop.myshoplaza.com/pages/about-us?template=abc&preview_theme_id=t1" {
		t.Errorf("no-session URL = %q", got)
	}
	got = buildPreviewURL("shop.myshoplaza.com", "/pages/about-us?template=abc", "t1", "sess-1", "")
	if !strings.HasPrefix(got, "https://shop.myshoplaza.com/pages/about-us?template=abc&") || strings.Count(got, "?") != 1 {
		t.Errorf("session URL must continue the existing query with &, got %q", got)
	}
	if !strings.Contains(got, "&oseid=sess-1") {
		t.Errorf("session URL missing oseid, got %q", got)
	}
}

func TestHelp_Preview(t *testing.T) {
	out := helpFor(t, "themes", "+preview")
	for _, want := range []string{"+preview", "--theme-id", "-t", "--oseid"} {
		if !strings.Contains(out, want) {
			t.Errorf("+preview help missing %q:\n%s", want, out)
		}
	}
}
