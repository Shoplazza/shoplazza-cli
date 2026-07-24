package themes

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// resolveServer fakes the two resolve-chain endpoints: GET /themes (published
// list) and GET /themes/{id}/doctree.
func resolveServer(t *testing.T, listCalls *atomic.Int32) *httptest.Server {
	t.Helper()
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/openapi/2026-01/themes":
			listCalls.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{
				"themes": []any{map[string]any{"id": "t_pub", "name": "Nova", "published": "1"}},
			})
		case strings.HasSuffix(r.URL.Path, "/doctree"):
			_ = json.NewEncoder(w).Encode(map[string]any{
				"templates": []any{
					map[string]any{"id": "d_index", "location": "index.liquid"},
					map[string]any{"id": "d_product", "location": "product.liquid"},
				},
				"configs": []any{map[string]any{"id": "d_cfg", "location": "settings_data.json"}},
			})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func TestResolveThemeAndDoc_DefaultTheme(t *testing.T) {
	var listCalls atomic.Int32
	srv := resolveServer(t, &listCalls)
	defer srv.Close()

	theme, docID, err := resolveThemeAndDoc(context.Background(), client.New(srv.URL), "", "index", "")
	if err != nil {
		t.Fatalf("resolveThemeAndDoc: %v", err)
	}
	if theme != "t_pub" || docID != "d_index" {
		t.Fatalf("got (%q, %q), want (t_pub, d_index)", theme, docID)
	}
	if listCalls.Load() != 1 {
		t.Errorf("themes list calls = %d, want 1", listCalls.Load())
	}
}

func TestResolveThemeAndDoc_ExplicitThemeSkipsList(t *testing.T) {
	var listCalls atomic.Int32
	srv := resolveServer(t, &listCalls)
	defer srv.Close()

	theme, docID, err := resolveThemeAndDoc(context.Background(), client.New(srv.URL), "t_x", "product", "")
	if err != nil {
		t.Fatalf("resolveThemeAndDoc: %v", err)
	}
	if theme != "t_x" || docID != "d_product" {
		t.Fatalf("got (%q, %q), want (t_x, d_product)", theme, docID)
	}
	if listCalls.Load() != 0 {
		t.Errorf("themes list calls = %d, want 0 (explicit --theme)", listCalls.Load())
	}
}

func TestResolveThemeAndDoc_FileFlagAndPluralGroup(t *testing.T) {
	var listCalls atomic.Int32
	srv := resolveServer(t, &listCalls)
	defer srv.Close()

	// --file config/settings_data.json resolves through the pluralized
	// "configs" doctree group.
	_, docID, err := resolveThemeAndDoc(context.Background(), client.New(srv.URL), "t_x", "", "config/settings_data.json")
	if err != nil {
		t.Fatalf("resolveThemeAndDoc: %v", err)
	}
	if docID != "d_cfg" {
		t.Fatalf("docID = %q, want d_cfg", docID)
	}
}

func TestResolveThemeAndDoc_TemplateNotFound(t *testing.T) {
	var listCalls atomic.Int32
	srv := resolveServer(t, &listCalls)
	defer srv.Close()

	_, _, err := resolveThemeAndDoc(context.Background(), client.New(srv.URL), "t_x", "landing", "")
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("err = %v, want validation ExitError", err)
	}
}

func TestTemplateLocation(t *testing.T) {
	cases := []struct {
		template, file string
		wantGroup      string
		wantLoc        string
		wantErr        bool
	}{
		{template: "index", wantGroup: "templates", wantLoc: "index.liquid"},
		{file: "templates/product.liquid", wantGroup: "templates", wantLoc: "product.liquid"},
		{file: "config/settings_data.json", wantGroup: "configs", wantLoc: "settings_data.json"},
		{template: "index", file: "templates/a.liquid", wantErr: true}, // mutually exclusive
		{wantErr: true}, // both missing
		{file: "not-a-theme-path.txt", wantErr: true},
	}
	for _, tc := range cases {
		group, loc, err := templateLocation(tc.template, tc.file)
		if tc.wantErr {
			if err == nil {
				t.Errorf("templateLocation(%q, %q): want error", tc.template, tc.file)
			}
			continue
		}
		if err != nil {
			t.Errorf("templateLocation(%q, %q): %v", tc.template, tc.file, err)
			continue
		}
		if group != tc.wantGroup || loc != tc.wantLoc {
			t.Errorf("templateLocation(%q, %q) = (%q, %q), want (%q, %q)",
				tc.template, tc.file, group, loc, tc.wantGroup, tc.wantLoc)
		}
	}
}

// fetchSections must unwrap the upstream double-data envelope and yield the
// {schemas, sections} payload; splitSections/areaOf follow the real response
// shape (page_sections + fixed sections group keyed by id).
func TestFetchSections_RealSampleShape(t *testing.T) {
	_, self, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(self), "testdata", "schemas_list_sample.json"))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		wantPath := "/openapi/2026-01/themes/edit-sessions/ose_1/files/doc_1/sections"
		if r.URL.Path != wantPath {
			t.Errorf("path = %s, want %s", r.URL.Path, wantPath)
		}
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write(raw) // sample keeps the upstream {"data":{"data":{...}}} nesting minus CLI envelope
	}))
	defer srv.Close()

	inner, err := fetchSections(context.Background(), client.New(srv.URL), "ose_1", "doc_1")
	if err != nil {
		t.Fatalf("fetchSections: %v", err)
	}
	if _, ok := inner["schemas"]; !ok {
		t.Fatal("inner payload lost the schemas group")
	}

	page, fixed := splitSections(inner)
	if len(page) != 4 {
		t.Errorf("page sections = %d, want 4 (trimmed sample)", len(page))
	}
	if len(fixed) != 3 {
		t.Errorf("fixed sections = %d, want 3", len(fixed))
	}
	wantAreas := map[string]string{"announcement": "global", "header": "header", "footer": "footer"}
	for _, f := range fixed {
		id := getString(f, "id")
		if got := areaOf(id); got != wantAreas[id] {
			t.Errorf("areaOf(%q) = %q, want %q", id, got, wantAreas[id])
		}
	}
}

func TestPbCustomID(t *testing.T) {
	cases := []struct {
		typ    string
		wantID string
		wantOK bool
	}{
		{"shoplazza://apps/page-builder/blocks/custom-1024", "1024", true},
		{"page-builder/blocks/custom-56125487021326335", "56125487021326335", true},
		{"shoplazza://apps/public/blocks/promotion_grid/56125487021326335", "", false},
		{"hero_slideshow", "", false},
	}
	for _, tc := range cases {
		id, ok := pbCustomID(tc.typ)
		if id != tc.wantID || ok != tc.wantOK {
			t.Errorf("pbCustomID(%q) = (%q, %v), want (%q, %v)", tc.typ, id, ok, tc.wantID, tc.wantOK)
		}
	}
}

func TestIsSessionNotFound(t *testing.T) {
	if isSessionNotFound(nil) {
		t.Error("nil must not classify as session-not-found")
	}
	// Design marker and the live-observed upstream code both classify.
	for _, msg := range []string{
		"SESSION_NOT_FOUND",
		`list section cards failed: status 404, body {"code":2,"errors":["b_invalid_themeid"]}`,
	} {
		if !isSessionNotFound(errors.New(msg)) {
			t.Errorf("isSessionNotFound(%q) = false, want true", msg)
		}
	}
	if isSessionNotFound(errors.New("network unreachable")) {
		t.Error("unrelated error must not classify as session-not-found")
	}
}
