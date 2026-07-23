package themes

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/spf13/cobra"

	"shoplazza-cli-v2/internal/client"
	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

func editFlags(t *testing.T, vals map[string]any) common.FlagSet {
	t.Helper()
	cmd := &cobra.Command{Use: "+edit"}
	cmd.Flags().String("template", "", "")
	cmd.Flags().String("file", "", "")
	cmd.Flags().String("theme", "", "")
	cmd.Flags().String("session", "", "")
	cmd.Flags().String("ops", "", "")
	cmd.Flags().Bool("promote", false, "")
	for k, v := range vals {
		if err := cmd.Flags().Set(k, fmt.Sprint(v)); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	return common.NewCobraFlagSet(cmd)
}

// editServer fakes the whole +edit endpoint family and records every write
// request (method+path+decoded body) in order.
type editServer struct {
	srv *httptest.Server

	mu          sync.Mutex
	writes      []map[string]any
	failResults map[int]string // batch entry index → non-success result string
	added       []string       // section ids "created" by add_section entries
	conflict    bool           // promote responds conflict=true
}

func newEditServer(t *testing.T) *editServer {
	t.Helper()
	es := &editServer{}
	es.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case p == "/openapi/2026-01/themes" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"themes": []any{map[string]any{"id": "t_pub"}}})
		case p == "/openapi/2026-01/shop":
			_ = json.NewEncoder(w).Encode(map[string]any{"shop": map[string]any{"domain": "unit.myshoplaza.com"}})
		case strings.HasSuffix(p, "/doctree"):
			_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{
				map[string]any{"id": "d_index", "location": "index.liquid"},
				map[string]any{"id": "d_product", "location": "product.liquid"},
			}})
		case p == "/openapi/2026-01/products" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{"handle": "demo-product"}}})
		case strings.HasSuffix(p, "/edit-sessions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"oseid": "ose_new"})
		case strings.HasSuffix(p, "/sections") && r.Method == http.MethodGet:
			pageSections := []any{
				map[string]any{"id": 111, "type": "hero_slideshow", "display": true, "settings": map[string]any{},
					"blocks": []any{map[string]any{"type": "slide", "settings": map[string]any{}}}},
				map[string]any{"id": 222, "type": "shoplazza://apps/page-builder/blocks/custom-9527", "display": true,
					"settings": map[string]any{}, "blocks": []any{}},
			}
			es.mu.Lock()
			for _, id := range es.added { // sections "created" by earlier batch adds
				pageSections = append(pageSections, map[string]any{"id": id, "type": "rich_text", "display": true,
					"settings": map[string]any{}, "blocks": []any{}})
			}
			es.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"schemas": map[string]any{
					"hero_slideshow": map[string]any{
						"max_blocks": 2,
						"blocks":     []any{map[string]any{"type": "slide", "name": map[string]any{"zh-CN": "幻灯"}}},
					},
				},
				"sections": map[string]any{
					"page_sections": pageSections,
					// dedicated header group (real df423620 shape — numeric ids, not the
					// semantic-id fixed group). Exercises sectionsByArea's group path.
					"header_sections": []any{
						map[string]any{"id": "hsec1", "type": "header_bar", "display": true, "settings": map[string]any{}, "blocks": []any{}},
					},
					"sections": []any{}, // truly-fixed cards (cart_drawer etc.)
				},
			}})
		case strings.HasSuffix(p, "/operations") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.mu.Lock()
			es.writes = append(es.writes, map[string]any{"method": r.Method, "path": p, "body": body})
			ops, _ := body["operations"].([]any)
			results := make([]any, 0, len(ops))
			for i, o := range ops {
				om, _ := o.(map[string]any)
				res := "success"
				if fr, ok := es.failResults[i]; ok {
					res = fr
				} else if om["op"] == "add_section" {
					es.added = append(es.added, fmt.Sprintf("sec_new%d", len(es.added)+1))
				}
				results = append(results, map[string]any{"op": om["op"], "result": res})
			}
			es.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": results}})
		case strings.HasSuffix(p, "/page-builder/blocks") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.mu.Lock()
			es.writes = append(es.writes, map[string]any{"method": r.Method, "path": p, "body": body})
			es.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"section": map[string]any{
				"type": "shoplazza://apps/page-builder/blocks/custom-9527/regenerated", "name": "pbcard",
				"settings": map[string]any{}, "blocks": []any{},
			}}})
		case strings.HasSuffix(p, "/promote"):
			if es.conflict { // real behavior: HTTP 409, not a {conflict:true} body
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"message":"edit session has conflict with draft, retry with force=true to overwrite"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"promoted": true})
		default: // anything else is unexpected under the batch-ops flow
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.mu.Lock()
			es.writes = append(es.writes, map[string]any{"method": r.Method, "path": p, "body": body})
			es.mu.Unlock()
			t.Errorf("unexpected request: %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return es
}

func editExec(t *testing.T, es *editServer, vals map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := editExecute(context.Background(), common.ExecInput{
		Flags:  editFlags(t, vals),
		Tool:   "edit",
		Client: client.New(es.srv.URL),
	})
	return res.Body, err
}

func TestEdit_FirstEditFlow(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"replace_props","target":"111","props":{"heading":"X"}},
		         {"op":"update_slot","target":"111.blocks[0]","props":{"title":"Y"}}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	if body["oseid"] != "ose_new" || body["session_created"] != true || body["promoted"] != false {
		t.Fatalf("body = %v", body)
	}
	applied := body["applied"].([]map[string]any)
	if len(applied) != 2 || applied[0]["result"] != "success" || applied[1]["result"] != "success" {
		t.Fatalf("applied = %v", applied)
	}
	preview := body["preview_url"].(string)
	if !strings.Contains(preview, "unit.myshoplaza.com") || !strings.Contains(preview, "oseid=ose_new") {
		t.Errorf("preview_url = %q", preview)
	}
	// The whole batch travels in ONE request; update_slot translates to the
	// server's replace_props with a dot-index block path.
	batch := editWriteBody(es, http.MethodPost, "/operations")
	if batch == nil {
		t.Fatal("batch-ops never sent")
	}
	ops := batch["operations"].([]any)
	if len(ops) != 2 {
		t.Fatalf("operations = %v", ops)
	}
	first := ops[0].(map[string]any)
	second := ops[1].(map[string]any)
	if first["op"] != "replace_props" || first["target"] != "111" {
		t.Errorf("op[0] = %v", first)
	}
	if second["op"] != "replace_props" || second["target"] != "111.blocks.0" {
		t.Errorf("op[1] = %v, want translated update_slot with dot path", second)
	}
}

func TestEdit_SessionReuseAndImplicitReadOnlyWhenNeeded(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	// Pure block-level props batch with --session: no create-session, no schemas-list.
	_, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_given",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	for _, wr := range es.writes {
		p := wr["path"].(string)
		if strings.HasSuffix(p, "/edit-sessions") || strings.HasSuffix(p, "/sections") && wr["method"] == "GET" {
			t.Errorf("unexpected call: %v", p)
		}
		if !strings.Contains(p, "ose_given") {
			t.Errorf("write not addressed to the given session: %v", p)
		}
	}
}

// TestEdit_PreviewPathByTemplate pins the preview_url path resolution:
// resource templates point at a representative item, static/index at /.
func TestEdit_PreviewPathByTemplate(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "product", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	preview := body["preview_url"].(string)
	if !strings.Contains(preview, "/products/demo-product?") {
		t.Errorf("preview_url = %q, want /products/demo-product path", preview)
	}

	body, err = editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	if preview := body["preview_url"].(string); !strings.Contains(preview, "unit.myshoplaza.com/?") {
		t.Errorf("index preview_url = %q, want homepage path", preview)
	}
}

// TestPreviewPageName covers the template/file → page-name extraction.
func TestPreviewPageName(t *testing.T) {
	cases := []struct{ template, file, want string }{
		{"index", "", "index"},
		{"product.custom", "", "product"},
		{"", "templates/page.summer.liquid", "page"},
		{"", "sections/foo.liquid", ""}, // non-templates group: no storefront page
		{"", "", ""},
	}
	for _, c := range cases {
		if got := previewPageName(c.template, c.file); got != c.want {
			t.Errorf("previewPageName(%q, %q) = %q, want %q", c.template, c.file, got, c.want)
		}
	}
}

// TestResolvePreviewPath_LocalOnly covers the paths that never touch the
// network: static pages, article, and unknown templates (nil client proves it).
func TestResolvePreviewPath_LocalOnly(t *testing.T) {
	cases := []struct{ template, want string }{
		{"index", ""},
		{"cart", "cart"},
		{"search", "search"},
		{"404", "404"},
		{"article", ""}, // needs a two-hop handle, unsupported → homepage
		{"whatever", ""},
	}
	for _, c := range cases {
		if got := resolvePreviewPath(context.Background(), nil, c.template, ""); got != c.want {
			t.Errorf("resolvePreviewPath(%q) = %q, want %q", c.template, got, c.want)
		}
	}
}

// TestFirstHandleIn covers list-response shapes: named key, data wrapper,
// generic slice, and the empty fallback.
func TestFirstHandleIn(t *testing.T) {
	cases := []struct {
		resp map[string]any
		want string
	}{
		{map[string]any{"products": []any{map[string]any{"handle": "p1"}}}, "p1"},
		{map[string]any{"data": map[string]any{"collections": []any{map[string]any{"handle": "c1"}}}}, "c1"},
		{map[string]any{"whatever": []any{map[string]any{"handle": "x1"}}}, "x1"},
		{map[string]any{"products": []any{}}, ""},
		{map[string]any{}, ""},
	}
	for _, c := range cases {
		if got := firstHandleIn(c.resp); got != c.want {
			t.Errorf("firstHandleIn(%v) = %q, want %q", c.resp, got, c.want)
		}
	}
}

func TestEdit_MixedBatchRoutesPbAndBackfillsBody(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}},
		         {"op":"update_pb","target":"222","ops":[{"action":"update","targetId":"0","settings":{}}]}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	applied := body["applied"].([]map[string]any)
	if len(applied) != 2 || applied[1]["result"] != "success" {
		t.Fatalf("applied = %v", applied)
	}
	// pb-block-save runs first and generates the replacement card.
	pb := editWriteBody(es, http.MethodPost, "/page-builder/blocks")
	if pb == nil {
		t.Fatal("update_pb never hit pb-block-save")
	}
	for k, want := range map[string]string{
		"event_type": "theme", "action": "save", "origin_template_id": "9527",
		"oseid": "ose_x", "doc_id": "d_index", "section_id": "222", "theme_id": "t_pub",
	} {
		if fmt.Sprint(pb[k]) != want {
			t.Errorf("pb body %s = %v, want %s", k, pb[k], want)
		}
	}
	// The batch swaps the card: remove_section old + add_section generated.
	batch := editWriteBody(es, http.MethodPost, "/operations")
	ops := batch["operations"].([]any)
	if len(ops) != 3 {
		t.Fatalf("operations = %v", ops)
	}
	rm := ops[1].(map[string]any)
	add := ops[2].(map[string]any)
	if rm["op"] != "remove_section" || rm["target"] != "222" {
		t.Errorf("op[1] = %v, want remove_section 222", rm)
	}
	card, _ := add["value"].(map[string]any)
	if add["op"] != "add_section" || card == nil || !strings.Contains(fmt.Sprint(card["type"]), "custom-9527/regenerated") {
		t.Errorf("op[2] = %v, want add_section with the generated card", add)
	}
	// The generated section's id is recovered from the re-read.
	if applied[1]["new_section_id"] != "sec_new1" {
		t.Errorf("new_section_id = %v", applied[1]["new_section_id"])
	}
}

func TestEdit_AppendValidatesAndEchoesNewTarget(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"append_array_item","target":"111.blocks","value":{"type":"slide","settings":{}}}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	applied := body["applied"].([]map[string]any)
	if applied[0]["new_target"] != "111.blocks[1]" { // container already holds 1 block
		t.Errorf("new_target = %v", applied[0]["new_target"])
	}

	// Second append would exceed max_blocks=2 (schema gate, client-side).
	_, err = editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"append_array_item","target":"111.blocks","value":{"type":"slide","settings":{}}},
		         {"op":"append_array_item","target":"111.blocks","value":{"type":"__bogus__","settings":{}}}]`})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("err = %v, want validation", err)
	}
}

func TestEdit_AddSectionEchoesNewSectionID(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"add_section","name":"rich_text"}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	applied := body["applied"].([]map[string]any)
	if applied[0]["new_section_id"] != "sec_new1" { // recovered by re-reading the session
		t.Errorf("new_section_id = %v", applied[0]["new_section_id"])
	}
}

// editWriteBody returns the decoded body of the first recorded write matching
// (method, path suffix).
func editWriteBody(es *editServer, method, pathSuffix string) map[string]any {
	es.mu.Lock()
	defer es.mu.Unlock()
	for _, wr := range es.writes {
		if wr["method"] == method && strings.HasSuffix(fmt.Sprint(wr["path"]), pathSuffix) {
			if b, ok := wr["body"].(map[string]any); ok {
				return b
			}
		}
	}
	return nil
}

func TestEdit_PositionTranslationAndPlacement(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	// move_section "position first" resolves against the section's own area
	// layout into the server's {position, move_target} pair.
	if _, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"move_section","target":"hsec1","position":"first"}]`}); err != nil {
		t.Fatalf("move header: %v", err)
	}
	batch := editWriteBody(es, http.MethodPost, "/operations")
	if batch == nil {
		t.Fatal("batch-ops never sent")
	}
	mv := batch["operations"].([]any)[0].(map[string]any)
	if mv["op"] != "move_section" || mv["position"] != "before" || mv["move_target"] != "hsec1" {
		t.Errorf("move entry = %v, want before/hsec1 (first of its area)", mv)
	}

	// add_section with a position: the server always appends, so the CLI
	// issues a follow-up move batch once the new id is recovered.
	es.mu.Lock()
	es.writes = nil
	es.mu.Unlock()
	body, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"add_section","name":"rich_text","position":"after:111"}]`})
	if err != nil {
		t.Fatalf("add after: %v", err)
	}
	var batches []map[string]any
	es.mu.Lock()
	for _, wr := range es.writes {
		if strings.HasSuffix(fmt.Sprint(wr["path"]), "/operations") {
			batches = append(batches, wr["body"].(map[string]any))
		}
	}
	es.mu.Unlock()
	if len(batches) != 2 {
		t.Fatalf("batches = %d, want add batch + placement batch", len(batches))
	}
	place := batches[1]["operations"].([]any)[0].(map[string]any)
	if place["op"] != "move_section" || place["target"] != "sec_new1" ||
		place["position"] != "after" || place["move_target"] != "111" {
		t.Errorf("placement move = %v", place)
	}
	if body["applied"].([]map[string]any)[0]["new_section_id"] != "sec_new1" {
		t.Errorf("new_section_id = %v", body["applied"])
	}
}

func TestEdit_ErrorCarriesExample(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	// update_slot missing props → validation error must carry a valid example.
	_, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"update_slot","target":"111.blocks[0]"}]`})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("err = %v, want validation ExitError", err)
	}
	if !strings.Contains(fmt.Sprint(exitErr.Envelope()["example"]), `"op":"update_slot"`) {
		t.Errorf("error example = %v, want an update_slot sample", exitErr.Envelope()["example"])
	}
}

// TestEdit_BatchPartialFailure pins the independent-application contract:
// one failed op does not stop the others; the envelope carries per-op results.
func TestEdit_BatchPartialFailure(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()
	es.failResults = map[int]string{1: "target_not_found"}

	_, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"1"}},
		         {"op":"replace_props","target":"222","props":{"b":"2"}},
		         {"op":"replace_props","target":"111","props":{"c":"3"}}]`})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("err = %v, want api ExitError", err)
	}
	env := exitErr.Envelope()
	if env["oseid"] != "ose_x" {
		t.Errorf("envelope oseid = %v", env["oseid"])
	}
	results := env["results"].([]map[string]any)
	if len(results) != 3 || results[0]["result"] != "success" ||
		results[1]["result"] != "target_not_found" || results[2]["result"] != "success" {
		t.Errorf("results = %v", results)
	}
	if fmt.Sprint(env["failed"]) != "[1]" {
		t.Errorf("failed = %v", env["failed"])
	}
	if !strings.Contains(fmt.Sprint(env["hint"]), "--session ose_x") {
		t.Errorf("hint = %v", env["hint"])
	}
	// The whole batch still went out in one request.
	batch := editWriteBody(es, http.MethodPost, "/operations")
	if n := len(batch["operations"].([]any)); n != 3 {
		t.Errorf("operations sent = %d, want 3", n)
	}
}

func TestEdit_PromoteAndConflict(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x", "promote": true,
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"1"}}]`})
	if err != nil {
		t.Fatalf("promote: %v", err)
	}
	if body["promoted"] != true {
		t.Errorf("promoted = %v", body["promoted"])
	}

	es.conflict = true
	_, err = editExec(t, es, map[string]any{"template": "index", "session": "ose_x", "promote": true,
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"1"}}]`})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("err = %v, want ExitError", err)
	}
	env := exitErr.Envelope()
	if env["conflict"] != true || env["preview_url"] == nil {
		t.Errorf("conflict envelope = %v", env)
	}
	if !strings.Contains(fmt.Sprint(env["hint"]), "force") {
		t.Errorf("hint = %v", env["hint"])
	}
}

func TestValidateOps_Table(t *testing.T) {
	cases := []struct {
		name    string
		ops     string
		wantErr string // substring; empty = valid
	}{
		{"unknown op", `[{"op":"nope","target":"s"}]`, "unknown op"},
		{"update_slot needs block target", `[{"op":"update_slot","target":"s","props":{"a":1}}]`, "block path"},
		{"replace_props needs props", `[{"op":"replace_props","target":"s"}]`, "props is required"},
		{"append needs container", `[{"op":"append_array_item","target":"s.blocks[0]","value":{"type":"x"}}]`, "container path"},
		{"add_section pb needs template_id", `[{"op":"add_section","pb":true}]`, "template_id"},
		{"move needs to_index", `[{"op":"move_section","target":"s"}]`, "to_index"},
		{"visibility needs visible", `[{"op":"set_visibility","target":"s"}]`, "visible is required"},
		{"update_pb needs inner ops", `[{"op":"update_pb","target":"s"}]`, "ops is required"},
		{"descending violation", `[{"op":"remove_array_item","target":"s.blocks[0]"},{"op":"remove_array_item","target":"s.blocks[1]"}]`, "descending"},
		{"descending ok", `[{"op":"remove_array_item","target":"s.blocks[1]"},{"op":"remove_array_item","target":"s.blocks[0]"}]`, ""},
		{"cross-container independent", `[{"op":"remove_array_item","target":"s.blocks[0].blocks[0]"},{"op":"remove_array_item","target":"s.blocks[1]"}]`, ""},
	}
	for _, tc := range cases {
		ops, err := parseOps([]byte(tc.ops))
		if err == nil {
			err = validateOps(ops)
		}
		if tc.wantErr == "" {
			if err != nil {
				t.Errorf("%s: unexpected error %v", tc.name, err)
			}
			continue
		}
		if err == nil || !strings.Contains(err.Error(), tc.wantErr) {
			t.Errorf("%s: err = %v, want substring %q", tc.name, err, tc.wantErr)
		}
	}
}

// TestValidateOps_PbTemplateHint pins the discovery hint on the
// pb-without-template_id error (custom templates surface via source=custom).
func TestValidateOps_PbTemplateHint(t *testing.T) {
	ops, err := parseOps([]byte(`[{"op":"add_section","pb":true}]`))
	if err != nil {
		t.Fatalf("parseOps: %v", err)
	}
	var exitErr *output.ExitError
	if !errors.As(validateOps(ops), &exitErr) {
		t.Fatal("want *output.ExitError")
	}
	env := exitErr.Envelope()
	if !strings.Contains(fmt.Sprint(env["hint"]), `"source":"custom"`) {
		t.Errorf("hint = %v, want list-card source=custom discovery", env["hint"])
	}
}

func TestEdit_DryRunZeroCall(t *testing.T) {
	res, err := editExecute(context.Background(), common.ExecInput{
		Flags: editFlags(t, map[string]any{"template": "index", "promote": true,
			"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}},
			         {"op":"update_pb","target":"222","ops":[{"action":"update","targetId":"0"}]}]`}),
		DryRun: true, // nil Client proves zero network
	})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	joined := ""
	for _, p := range res.Plans {
		joined += fmt.Sprintf("%s %s %v\n", p.Method, p.Path, p.Body) // %v keeps <placeholders> unescaped
	}
	for _, want := range []string{phThemeID, phDocID, phOseid, phCustomID, "/promote"} {
		if !strings.Contains(joined, want) {
			t.Errorf("plans missing %q:\n%s", want, joined)
		}
	}
}

func TestSnapshot_EditDryRun(t *testing.T) {
	res, err := editExecute(context.Background(), common.ExecInput{
		Flags: editFlags(t, map[string]any{"template": "index",
			"ops": `[{"op":"update_slot","target":"111.blocks[0]","props":{"title":"X"}},
			         {"op":"remove_array_item","target":"111.blocks[1]"},
			         {"op":"append_array_item","target":"111.blocks","value":{"type":"slide","settings":{}}},
			         {"op":"update_pb","target":"222","ops":[{"action":"update","targetId":"0","settings":{}}]}]`}),
		DryRun: true,
	})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	snapshot(t, "edit_dry_run", plansToMap(res.Plans))
}

func TestHelp_Edit(t *testing.T) {
	out := helpFor(t, "themes", "+edit")
	for _, want := range []string{"+edit", "--ops", "--session", "--promote", "apply and persist independently", "update_pb"} {
		if !strings.Contains(out, want) {
			t.Errorf("+edit help missing %q:\n%s", want, out)
		}
	}
}
