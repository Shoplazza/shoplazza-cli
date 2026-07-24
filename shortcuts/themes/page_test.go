package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// pageFlags builds a FlagSet mirroring the +page flag declarations.
func pageFlags(t *testing.T, vals map[string]any) common.FlagSet {
	t.Helper()
	cmd := &cobra.Command{Use: "+page"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("theme", "", "")
	cmd.Flags().String("session", "", "")
	cmd.Flags().String("area", "all", "")
	cmd.Flags().String("section", "", "")
	cmd.Flags().String("include", "", "")
	cmd.Flags().Bool("list", false, "")
	for k, v := range vals {
		if err := cmd.Flags().Set(k, fmt.Sprint(v)); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	return common.NewCobraFlagSet(cmd)
}

// pageServerCounters tracks which endpoints the fake server saw.
type pageServerCounters struct {
	list, createSession, schemasList, pbGet, listTemplates atomic.Int32
}

// pageServer fakes the full +page endpoint family. sectionsPayload is served
// verbatim for schemas-list (defaults to the real dev-store sample).
func pageServer(t *testing.T, counters *pageServerCounters, sectionsPayload []byte) *httptest.Server {
	t.Helper()
	if sectionsPayload == nil {
		raw, err := os.ReadFile(filepath.Join(testdataDir(), "schemas_list_sample.json"))
		if err != nil {
			t.Fatalf("read sample: %v", err)
		}
		sectionsPayload = raw
	}
	return httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case p == "/openapi/2026-01/themes":
			counters.list.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"themes": []any{map[string]any{"id": "t_pub"}}})
		case strings.HasSuffix(p, "/doctree"):
			_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{
				map[string]any{"id": "d_index", "location": "index.liquid"},
				map[string]any{"id": "d_custom", "location": "page.summer.liquid"},
			}})
		case strings.HasSuffix(p, "/edit-sessions") && r.Method == http.MethodPost:
			counters.createSession.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"oseid": "ose_test"})
		case strings.HasSuffix(p, "/sections"):
			counters.schemasList.Add(1)
			_, _ = w.Write(sectionsPayload)
		case strings.Contains(p, "/page-builder/custom-templates/"):
			counters.pbGet.Add(1)
			if strings.HasSuffix(p, "/9500") { // sentinel id: this card 500s (fan-out isolation)
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"record not found"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"text": "#0 root \"Canvas\""}})
		case strings.HasSuffix(p, "/theme-templates"):
			counters.listTemplates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"theme_templates": []any{
				map[string]any{"type": "page", "suffix": "summer", "title": "夏日大促", "obj_title": "夏日大促页", "updated_at": "2026-07-01T10:00:00Z"},
			}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
}

func pageExec(t *testing.T, srv *httptest.Server, vals map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := pageExecute(context.Background(), common.ExecInput{
		Flags:  pageFlags(t, vals),
		Tool:   "page",
		Client: client.New(srv.URL),
	})
	return res.Body, err
}

func TestPage_MainRead_CreatesSessionAndFlattens(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, nil)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "area": "page"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	if body["oseid"] != "ose_test" || body["session_created"] != true {
		t.Fatalf("session echo = (%v, %v), want (ose_test, true)", body["oseid"], body["session_created"])
	}
	if c.createSession.Load() != 1 {
		t.Errorf("create-session calls = %d, want 1", c.createSession.Load())
	}
	rows := body["sections"].([]map[string]any)
	if len(rows) != 4 { // trimmed sample keeps 4 page cards
		t.Fatalf("sections = %d, want 4", len(rows))
	}
	first := rows[0]
	if first["section_id"] != "1638950411341" || first["type"] != "hero_slideshow" || first["visible"] != true {
		t.Errorf("first row = %v", first)
	}
	blocks := first["blocks"].([]map[string]any)
	if len(blocks) != 2 {
		t.Fatalf("first row blocks = %d, want 2", len(blocks))
	}
	if _, err := parseTarget(blocks[0]["target"].(string)); err != nil {
		t.Errorf("pre-built target does not parse: %v", err)
	}
	areas := body["areas"].(map[string]any)
	if areas["header"] != 1 || areas["footer"] != 1 || areas["global"] != 1 {
		t.Errorf("areas = %v, want 1/1/1", areas)
	}
}

func TestPage_SessionReuseSkipsCreate(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, nil)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "session": "ose_given"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	if body["oseid"] != "ose_given" || body["session_created"] != false {
		t.Fatalf("session echo = (%v, %v), want (ose_given, false)", body["oseid"], body["session_created"])
	}
	if c.createSession.Load() != 0 {
		t.Errorf("create-session calls = %d, want 0", c.createSession.Load())
	}
}

func TestPage_AreaSelection(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, nil)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "area": "header"})
	if err != nil {
		t.Fatalf("area header: %v", err)
	}
	rows := body["sections"].([]map[string]any)
	if len(rows) != 1 || rows[0]["section_id"] != "header" {
		t.Fatalf("header rows = %v", rows)
	}

	body, err = pageExec(t, srv, map[string]any{"template": "index", "area": "all"})
	if err != nil {
		t.Fatalf("area all: %v", err)
	}
	all := body["sections"].([]map[string]any) // flat, each row carries its area
	seen := map[string]bool{}
	for _, r := range all {
		seen[getString(r, "area")] = true
	}
	for _, a := range []string{"page", "header", "footer", "global"} {
		if !seen[a] {
			t.Errorf("area all: no section tagged area=%q (rows=%v)", a, all)
		}
	}
	areas := body["areas"].(map[string]any)
	if areas["page"] == nil || areas["header"] != 1 || areas["footer"] != 1 || areas["global"] != 1 {
		t.Errorf("area all areas count = %v", areas)
	}
}

func TestBuildSectionRow_Visibility(t *testing.T) {
	cases := []struct {
		m    map[string]any
		want bool
	}{
		{map[string]any{"id": 1, "display": true}, true},                    // default visible
		{map[string]any{"id": 1, "display": true, "disabled": true}, false}, // set_visibility hides via disabled
		{map[string]any{"id": 1, "display": false}, false},                  // display off
	}
	for _, c := range cases {
		if got := buildSectionRow(c.m)["visible"]; got != c.want {
			t.Errorf("visible for %v = %v, want %v", c.m, got, c.want)
		}
	}
}

// TestBuildSectionRow_Name pins the per-kind name source: pb cards read
// schema.name, theme/app cards read name, both passed through verbatim;
// fixed cards have none and the field is omitted.
func TestBuildSectionRow_Name(t *testing.T) {
	bilingual := map[string]any{"en-US": "Static text 2", "zh-CN": "组合轮播2"}
	cases := []struct {
		label string
		m     map[string]any
		want  any // nil = field absent
	}{
		{"pb card takes schema.name", map[string]any{
			"id": 1, "type": "shoplazza://apps/page-builder/blocks/global-666/abc",
			"name": "image_with_text", "schema": map[string]any{"name": bilingual},
		}, bilingual},
		{"app card takes name", map[string]any{
			"id": 2, "type": "shoplazza://apps/public/blocks/video_hero/41113",
			"name": map[string]any{"en-US": "Video hero", "zh-CN": "视频背景"},
		}, map[string]any{"en-US": "Video hero", "zh-CN": "视频背景"}},
		{"theme card takes name", map[string]any{
			"id": 3, "type": "hero_slideshow", "name": "hero_slideshow",
		}, "hero_slideshow"},
		{"fixed card has no name", map[string]any{
			"id": "header", "type": "header",
		}, nil},
	}
	for _, c := range cases {
		got, ok := buildSectionRow(c.m)["name"]
		if c.want == nil {
			if ok {
				t.Errorf("%s: name = %v, want field absent", c.label, got)
			}
			continue
		}
		if fmt.Sprint(got) != fmt.Sprint(c.want) {
			t.Errorf("%s: name = %v, want %v", c.label, got, c.want)
		}
	}
}

// TestSectionsByArea_GlobalSectionsKey covers the renamed fixed-cards group:
// newer schemas-list responses use "global_sections" instead of "sections".
func TestSectionsByArea_GlobalSectionsKey(t *testing.T) {
	inner := map[string]any{"sections": map[string]any{
		"page_sections": []any{map[string]any{"id": 111.0, "type": "rich_text"}},
		"global_sections": []any{
			map[string]any{"id": "announcement", "type": "announcement"},
			map[string]any{"id": "header", "type": "header"},
			map[string]any{"id": "footer", "type": "footer"},
		},
	}}
	ba := sectionsByArea(inner)
	if len(ba["page"]) != 1 || len(ba["header"]) != 1 || len(ba["footer"]) != 1 || len(ba["global"]) != 1 {
		t.Fatalf("area sizes = page:%d header:%d footer:%d global:%d, want 1/1/1/1",
			len(ba["page"]), len(ba["header"]), len(ba["footer"]), len(ba["global"]))
	}
	if anyToString(ba["global"][0]["id"]) != "announcement" {
		t.Errorf("global bucket = %v", ba["global"])
	}
}

func TestPage_IncludeSchemaProjection(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, nil)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "include": "schema"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	schema := body["schema"].(map[string]any)
	hero := schema["hero_slideshow"].(map[string]any)
	if fmt.Sprint(hero["max_blocks"]) != "5" { // client decodes numbers as json.Number
		t.Errorf("max_blocks = %v, want 5", hero["max_blocks"])
	}
	settings := hero["settings"].([]map[string]any)
	if len(settings) == 0 || settings[0]["label"] != "全屏宽度" {
		t.Errorf("zh-CN projection failed: %v", settings)
	}
	if _, ok := hero["blocks"].(map[string]any)["slide"]; !ok {
		t.Errorf("sub-block schema missing: %v", hero["blocks"])
	}
}

// pbSectionsPayload crafts a sections payload with one PB custom card.
func pbSectionsPayload() []byte {
	payload := map[string]any{"data": map[string]any{
		"schemas": map[string]any{},
		"sections": map[string]any{
			"page_sections": []any{
				map[string]any{"id": 111.0, "type": "hero_slideshow", "display": true, "settings": map[string]any{}, "blocks": []any{}},
				map[string]any{"id": 222.0, "type": "shoplazza://apps/page-builder/blocks/custom-9527", "display": true, "settings": map[string]any{}, "blocks": []any{}},
			},
			"sections": []any{},
		},
	}}
	b, _ := json.Marshal(payload)
	return b
}

func TestPage_IncludePbExpandsCanvas(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, pbSectionsPayload())
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "include": "pb"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	rows := body["sections"].([]map[string]any)
	pb := rows[1]
	if pb["kind"] != "pb" {
		t.Fatalf("pb row not tagged: %v", pb)
	}
	if pb["canvas"] != "#0 root \"Canvas\"" {
		t.Errorf("canvas = %v", pb["canvas"])
	}
	if c.pbGet.Load() != 1 {
		t.Errorf("pb-blocks-get calls = %d, want 1", c.pbGet.Load())
	}
	if rows[0]["canvas"] != nil {
		t.Errorf("theme card must not get a canvas")
	}
}

// TestPage_PbFanoutIsolation covers the concurrent pb-blocks-get fan-out:
// every PB card is fetched, and one card's failure degrades to canvas_error
// on that row only.
func TestPage_PbFanoutIsolation(t *testing.T) {
	payload := map[string]any{"data": map[string]any{
		"schemas": map[string]any{},
		"sections": map[string]any{
			"page_sections": []any{
				map[string]any{"id": 222.0, "type": "shoplazza://apps/page-builder/blocks/custom-9527", "display": true, "settings": map[string]any{}, "blocks": []any{}},
				map[string]any{"id": 333.0, "type": "shoplazza://apps/page-builder/blocks/custom-9528", "display": true, "settings": map[string]any{}, "blocks": []any{}},
				map[string]any{"id": 444.0, "type": "shoplazza://apps/page-builder/blocks/custom-9500", "display": true, "settings": map[string]any{}, "blocks": []any{}},
			},
			"sections": []any{},
		},
	}}
	raw, _ := json.Marshal(payload)

	var c pageServerCounters
	srv := pageServer(t, &c, raw)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "include": "pb"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	rows := body["sections"].([]map[string]any)
	if len(rows) != 3 {
		t.Fatalf("sections = %d, want 3", len(rows))
	}
	for _, row := range rows[:2] {
		if row["canvas"] != "#0 root \"Canvas\"" || row["canvas_error"] != nil {
			t.Errorf("row %v: canvas = %v, canvas_error = %v", row["section_id"], row["canvas"], row["canvas_error"])
		}
	}
	if rows[2]["canvas"] != nil || rows[2]["canvas_error"] == nil {
		t.Errorf("failing card: canvas = %v, canvas_error = %v", rows[2]["canvas"], rows[2]["canvas_error"])
	}
	if c.pbGet.Load() != 3 {
		t.Errorf("pb-blocks-get calls = %d, want 3", c.pbGet.Load())
	}
}

// TestPage_ListDoctreeErrorPrecedence pins the error priority of the
// concurrent --list pair: a doctree failure surfaces even when
// list-templates succeeds (matching the former serial order).
func TestPage_ListDoctreeErrorPrecedence(t *testing.T) {
	var listTemplates atomic.Int32
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.URL.Path == "/openapi/2026-01/themes":
			_ = json.NewEncoder(w).Encode(map[string]any{"themes": []any{map[string]any{"id": "t_pub"}}})
		case strings.HasSuffix(r.URL.Path, "/doctree"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"doctree-boom"}`))
		case strings.HasSuffix(r.URL.Path, "/theme-templates"):
			listTemplates.Add(1)
			_ = json.NewEncoder(w).Encode(map[string]any{"theme_templates": []any{}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, r.URL.Path)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	defer srv.Close()

	_, err := pageExec(t, srv, map[string]any{"list": true})
	if err == nil || !strings.Contains(err.Error(), "doctree-boom") {
		t.Fatalf("err = %v, want the doctree error to win", err)
	}
	if listTemplates.Load() != 1 {
		t.Errorf("list-templates calls = %d, want 1 (pair still fires)", listTemplates.Load())
	}
}

func TestPage_SectionFocusAutoPb(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, pbSectionsPayload())
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "section": "222"})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	rows := body["sections"].([]map[string]any)
	if len(rows) != 1 || rows[0]["section_id"] != "222" {
		t.Fatalf("focus rows = %v", rows)
	}
	if rows[0]["canvas"] == nil {
		t.Error("focusing a PB card must auto-expand its canvas")
	}
}

func TestPage_List(t *testing.T) {
	var c pageServerCounters
	srv := pageServer(t, &c, nil)
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"list": true})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	templates := body["templates"].([]map[string]any)
	var std, custom map[string]any
	for _, tpl := range templates {
		switch tpl["template"] {
		case "index":
			std = tpl
		case "page.summer":
			custom = tpl
		}
	}
	if std == nil || std["type"] != "system" || std["title"] != "首页" || std["url"] != "/" {
		t.Errorf("standard entry = %v", std)
	}
	if custom == nil || custom["title"] != "夏日大促" || custom["updated_at"] == nil {
		t.Errorf("custom entry = %v", custom)
	}
	if c.listTemplates.Load() != 1 {
		t.Errorf("list-templates calls = %d, want 1", c.listTemplates.Load())
	}
}

func TestPage_ListMutualExclusion(t *testing.T) {
	res, err := pageExecute(context.Background(), common.ExecInput{
		Flags: pageFlags(t, map[string]any{"list": true, "area": "header"}),
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("err = %v (res=%v), want validation ExitError", err, res)
	}
}

// Dry-run must plan every intended request with placeholders and touch no
// network (nil Client proves zero-call).
func TestPage_DryRunZeroCall(t *testing.T) {
	res, err := pageExecute(context.Background(), common.ExecInput{
		Flags:  pageFlags(t, map[string]any{"template": "index", "include": "schema,pb"}),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	if len(res.Plans) != 5 {
		t.Fatalf("plans = %d, want 5", len(res.Plans))
	}
	joined := ""
	for _, p := range res.Plans {
		joined += p.Method + " " + p.Path + "\n"
	}
	for _, ph := range []string{phThemeID, phDocID, phOseid, phCustomID} {
		if !strings.Contains(joined, ph) {
			t.Errorf("plans missing placeholder %s:\n%s", ph, joined)
		}
	}
}

func TestSnapshot_PageDryRun(t *testing.T) {
	res, err := pageExecute(context.Background(), common.ExecInput{
		Flags:  pageFlags(t, map[string]any{"template": "index", "include": "schema,pb"}),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("pageExecute: %v", err)
	}
	snapshot(t, "page_dry_run", plansToMap(res.Plans))
}

func TestHelp_Page(t *testing.T) {
	out := helpFor(t, "themes", "+page")
	for _, want := range []string{"+page", "--template", "--session", "--area", "--include", "--list", "target"} {
		if !strings.Contains(out, want) {
			t.Errorf("+page help missing %q:\n%s", want, out)
		}
	}
}

// Theme-baked PB cards reference designer-side templates that 404 on the
// merchant store — canvas expansion must degrade per card, not fail the read.
func TestPage_IncludePbDegradesOnMissingTemplate(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case strings.HasSuffix(r.URL.Path, "/doctree"):
			_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{map[string]any{"id": "d_index", "location": "index.liquid"}}})
		case strings.Contains(r.URL.Path, "/custom-templates/"):
			w.WriteHeader(http.StatusInternalServerError)
			_, _ = w.Write([]byte(`{"message":"get custom template failed: status 404"}`))
		case strings.HasSuffix(r.URL.Path, "/sections"):
			_, _ = w.Write(pbSectionsPayload())
		default:
			t.Errorf("unexpected request: %s", r.URL.Path)
		}
	}))
	defer srv.Close()

	body, err := pageExec(t, srv, map[string]any{"template": "index", "theme": "t_x", "session": "ose_x", "include": "pb"})
	if err != nil {
		t.Fatalf("degrade must not fail the read: %v", err)
	}
	rows := body["sections"].([]map[string]any)
	pb := rows[1]
	if pb["canvas"] != nil {
		t.Errorf("canvas must be absent, got %v", pb["canvas"])
	}
	if ce, _ := pb["canvas_error"].(string); !strings.Contains(ce, "unavailable") {
		t.Errorf("canvas_error = %v", pb["canvas_error"])
	}
}
