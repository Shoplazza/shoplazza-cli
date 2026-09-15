package themes

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
)

// TestResolvePreviewPath_BoundCustomTemplate: a custom template bound to an
// object previews that object, not the store's first one — by handle
// (collections) and by url (pages carry no handle). An unbound template still
// falls back to the representative item.
func TestResolvePreviewPath_BoundCustomTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch p := r.URL.Path; {
		case strings.HasSuffix(p, "/theme-templates"):
			typ := r.URL.Query().Get("type")
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"theme_templates": []any{
				map[string]any{"suffix": "bound", "type": typ, "obj_id": "obj-9"},
				map[string]any{"suffix": "unbound", "type": typ, "obj_id": nil},
			}}})
		case p == "/openapi/2026-01/collections/obj-9":
			_ = json.NewEncoder(w).Encode(map[string]any{"collection": map[string]any{"id": "obj-9", "handle": "new-arrivals"}})
		case p == "/openapi/2026-01/pages/obj-9":
			_ = json.NewEncoder(w).Encode(map[string]any{"page": map[string]any{"id": "obj-9", "url": "/pages/about-us"}})
		case p == "/openapi/2026-01/collections":
			_ = json.NewEncoder(w).Encode(map[string]any{"collections": []any{map[string]any{"handle": "first-one"}}})
		case p == "/openapi/2026-01/pages":
			_ = json.NewEncoder(w).Encode(map[string]any{"pages": []any{map[string]any{"url": "/pages/first-page"}}})
		default:
			t.Errorf("unexpected request %s", p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()
	c := client.New(srv.URL)

	cases := []struct{ template, want string }{
		{"collection.bound", "collections/new-arrivals?template=bound"},
		{"page.bound", "pages/about-us?template=bound"}, // detail has url, no handle
		{"collection.unbound", "collections/first-one?template=unbound"},
		{"page.unbound", "pages/first-page?template=unbound"},
	}
	for _, tc := range cases {
		if got := resolvePreviewPath(context.Background(), c, "t1", tc.template, ""); got != tc.want {
			t.Errorf("resolvePreviewPath(%q) = %q, want %q", tc.template, got, tc.want)
		}
	}
}
