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
	"sync"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const testGenSchema = `{% schema %}
{"name":"card","settings":[{"id":"title","type":"text","default":"Hello"},{"id":"subtitle","type":"text","default":"Sub"}],
 "presets":[{"name":"card","cname":{"en-US":"Card","zh-CN":"卡片"},"settings":{"title":"Hello","subtitle":"Sub"}}]}
{% endschema %}`

// blockServer fakes the gen-blocks family plus the page endpoints the two
// shortcuts orchestrate, recording every write in order.
type blockServer struct {
	srv *httptest.Server

	mu     sync.Mutex
	writes []map[string]any
	// failResults keys the op sequence across every batch of one call, so a
	// case reads the same whether the ops travel in one request or several.
	failResults map[int]string
	opSeq       int
	revertFails bool
	branched    bool
	added       []string
}

func genSchema(cardType string) map[string]any {
	return map[string]any{
		"name": "card", "type": cardType,
		"settings": []any{
			map[string]any{"id": "title", "type": "text", "default": "Hello"},
			map[string]any{"id": "subtitle", "type": "text", "default": "Sub"},
		},
		"presets": []any{map[string]any{"name": "card", "cname": map[string]any{"en-US": "Card", "zh-CN": "卡片"},
			"settings": map[string]any{"title": "Hello", "subtitle": "Sub"}}},
	}
}

func newBlockServer(t *testing.T) *blockServer {
	t.Helper()
	bs := &blockServer{}
	record := func(r *http.Request, body map[string]any) {
		bs.mu.Lock()
		bs.writes = append(bs.writes, map[string]any{"method": r.Method, "path": r.URL.Path, "body": body})
		bs.mu.Unlock()
	}
	bs.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		p := r.URL.Path
		switch {
		case p == "/openapi/2026-01/themes" && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"themes": []any{map[string]any{"id": "t_pub"}}})
		case p == "/openapi/2026-01/shop":
			_ = json.NewEncoder(w).Encode(map[string]any{"shop": map[string]any{"domain": "unit.myshoplaza.com"}})
		case p == "/openapi/2026-01/products":
			_ = json.NewEncoder(w).Encode(map[string]any{"products": []any{map[string]any{"handle": "demo-product"}}})
		case strings.HasSuffix(p, "/doctree"):
			_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{
				map[string]any{"id": "d_index", "location": "index.liquid"},
				map[string]any{"id": "d_product", "location": "product.liquid"},
			}})
		case strings.HasSuffix(p, "/sections") && r.Method == http.MethodGet:
			page := []any{
				map[string]any{"id": 111, "type": "hero_slideshow", "display": true, "settings": map[string]any{},
					"blocks": []any{
						map[string]any{"type": "slide", "settings": map[string]any{}},
						map[string]any{"type": "blocks/gen_aaa", "settings": map[string]any{"title": "cur", "old_key": "x"}},
					}},
				map[string]any{"id": 222, "type": "blog_list", "display": true, "settings": map[string]any{}, "blocks": []any{}},
			}
			bs.mu.Lock()
			for _, id := range bs.added {
				page = append(page, map[string]any{"id": id, "type": "_blocks", "display": true, "settings": map[string]any{},
					"blocks": []any{map[string]any{"type": "blocks/gen_new", "settings": map[string]any{}}}})
			}
			bs.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"schemas":  map[string]any{"hero_slideshow": map[string]any{"blocks": []any{map[string]any{"type": "slide"}}}},
				"sections": map[string]any{"page_sections": page, "sections": []any{}},
			}})
		case strings.HasSuffix(p, "/gen-blocks") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			record(r, body)
			if strings.Contains(getString(body, "content"), "BAD") {
				w.WriteHeader(http.StatusUnprocessableEntity)
				_, _ = w.Write([]byte(`{"code":"InvalidParameter","message":"json_parse_error"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"revert_id": "rev_create", "settings": genSchema("blocks/gen_new")}})
		case strings.HasSuffix(p, "/gen-blocks") && r.Method == http.MethodPatch:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			record(r, body)
			typ := getString(mapField(body, "settings"), "type")
			resp := map[string]any{"revert_id": "rev_update", "settings": genSchema(typ)}
			if bs.branched {
				resp["branched"] = true
				resp["settings"] = genSchema("blocks/gen_bbb")
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": resp})
		case strings.HasSuffix(p, "/gen-blocks/revert") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			record(r, body)
			if bs.revertFails {
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"code":"Conflict","message":"snapshot gone"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"revert_id": "rev_undo"}})
		case strings.HasSuffix(p, "/gen-blocks") && r.Method == http.MethodGet:
			q := r.URL.Query()
			record(r, map[string]any{"type": q.Get("type"), "with_content": q.Get("with_content")})
			if q.Get("type") != "blocks/gen_aaa" {
				w.WriteHeader(http.StatusNotFound)
				_, _ = w.Write([]byte(`{"code":"ResourceNotFound","message":"Record not found"}`))
				return
			}
			doc := map[string]any{"id": "f1", "type": "blocks", "location": "gen_aaa.liquid", "hash": "h1"}
			if q.Get("with_content") == "true" {
				doc["content"] = "<div></div>\n" + testGenSchema
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"doc": doc, "saved": true, "ref_count": 3,
				"instances": []any{
					map[string]any{"index": []any{"111.blocks.1", "222.blocks.0"}},
					map[string]any{"product": []any{"333.blocks.0"}},
				},
			}})
		case strings.HasSuffix(p, "/operations") && r.Method == http.MethodPost:
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			record(r, body)
			ops, _ := body["operations"].([]any)
			results := make([]any, 0, len(ops))
			bs.mu.Lock()
			for _, o := range ops {
				om, _ := o.(map[string]any)
				res := "success"
				i := bs.opSeq
				bs.opSeq++
				if fr, ok := bs.failResults[i]; ok {
					res = fr
				} else if om["op"] == "add_section" {
					sid := getString(om, "section_id") // the server honours a supplied id
					if sid == "" {
						sid = fmt.Sprintf("sec_new%d", len(bs.added)+1)
					}
					bs.added = append(bs.added, sid)
				}
				results = append(results, map[string]any{"op": om["op"], "result": res})
			}
			bs.mu.Unlock()
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{"data": results}})
		default:
			t.Errorf("unexpected request: %s %s", r.Method, p)
			w.WriteHeader(http.StatusNotFound)
		}
	}))
	return bs
}

func (bs *blockServer) writesTo(suffix, method string) []map[string]any {
	bs.mu.Lock()
	defer bs.mu.Unlock()
	var out []map[string]any
	for _, wr := range bs.writes {
		if strings.HasSuffix(wr["path"].(string), suffix) && wr["method"] == method {
			out = append(out, wr)
		}
	}
	return out
}

// operations flattens every batch of one call in order — the split between
// batches is asserted by opBatches where it matters.
func (bs *blockServer) operations(t *testing.T) []map[string]any {
	t.Helper()
	var out []map[string]any
	for _, batch := range bs.opBatches(t) {
		out = append(out, batch...)
	}
	return out
}

func (bs *blockServer) opBatches(t *testing.T) [][]map[string]any {
	t.Helper()
	var out [][]map[string]any
	for _, wr := range bs.writesTo("/operations", http.MethodPost) {
		out = append(out, mapSlice(mapField(wr, "body")["operations"]))
	}
	return out
}

func writeTempLiquid(t *testing.T, content string) string {
	t.Helper()
	p := filepath.Join(t.TempDir(), "card.liquid")
	if err := os.WriteFile(p, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
	return p
}

func blockEditExec(t *testing.T, bs *blockServer, vals map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := blockEditExecute(context.Background(), common.ExecInput{Flags: shortcutFlags(t, blockEditShortcut, vals), Tool: "edit", Client: client.New(bs.srv.URL)})
	return res.Body, err
}

func blockGetExec(t *testing.T, bs *blockServer, vals map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := blockGetExecute(context.Background(), common.ExecInput{Flags: shortcutFlags(t, blockGetShortcut, vals), Tool: "get", Client: client.New(bs.srv.URL)})
	return res.Body, err
}

// wantValidation asserts err is a validation-exit error; an empty contains
// skips the message check.
func wantValidation(t *testing.T, err error, contains string) {
	t.Helper()
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
		t.Fatalf("want validation error, got %v", err)
	}
	if !strings.Contains(err.Error(), contains) {
		t.Errorf("error %q does not mention %q", err.Error(), contains)
	}
}

// ─────────── +edit ───────────

func TestBlockEdit_CreateAndPlace(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["type"] != "blocks/gen_new" || body["revert_id"] != "rev_create" || body["branched"] != false {
		t.Errorf("unexpected body: %v", body)
	}
	posts := bs.writesTo("/gen-blocks", http.MethodPost)
	if len(posts) != 1 || !strings.Contains(getString(mapField(posts[0], "body"), "content"), "{% schema %}") {
		t.Errorf("create body not sent: %v", posts)
	}
	ops := bs.operations(t)
	if len(ops) != 1 || ops[0]["op"] != "append_array_item" || ops[0]["target"] != "111.blocks" {
		t.Fatalf("ops: %v", ops)
	}
	value := mapField(ops[0], "value")
	if value["type"] != "blocks/gen_new" || mapField(value, "settings")["title"] != "Hello" {
		t.Errorf("append value: %v", value)
	}
	inst := mapField(body, "instance")
	if inst["template"] != "index" || inst["target"] != "111.blocks[2]" || inst["section_created"] != false {
		t.Errorf("instance: %v", inst)
	}
	if u := getString(body, "preview_url"); !strings.Contains(u, "oseid=ose_x") || !strings.Contains(u, "unit.myshoplaza.com") {
		t.Errorf("preview_url: %q", u)
	}
	if body["ops"] != nil {
		t.Errorf("ops should be nil when --ops is absent: %v", body["ops"])
	}
}

func TestBlockEdit_CreateWithOpsAppendsReplaceProps(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks",
		"ops": `{"title":"x"}`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ops := bs.operations(t)
	if len(ops) != 2 || ops[1]["op"] != "replace_props" || ops[1]["target"] != "111.blocks.2" || mapField(ops[1], "props")["title"] != "x" {
		t.Errorf("ops: %v", ops)
	}
	if mapField(body, "ops")["title"] != "x" {
		t.Errorf("ops echo: %v", body["ops"])
	}
}

// TestBlockEdit_CreateWithoutTargetAddsSectionThenAppends: creating always ends
// in an append. With no --target the CLI adds an empty "_blocks" section under
// an id it picks, so the append in the same batch can address it.
func TestBlockEdit_CreateWithoutTargetAddsSectionThenAppends(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ops := bs.operations(t)
	if len(ops) != 2 {
		t.Fatalf("want add_section + append_array_item, got %v", ops)
	}
	sid := getString(ops[0], "section_id")
	value := mapField(ops[0], "value")
	if ops[0]["op"] != "add_section" || sid == "" || value["type"] != "_blocks" {
		t.Errorf("add_section: %v", ops[0])
	}
	if blocks := mapSlice(value["blocks"]); len(blocks) != 0 {
		t.Errorf("the added section must be an empty shell, got blocks %v", blocks)
	}
	if ops[1]["op"] != "append_array_item" || ops[1]["target"] != sid+".blocks" {
		t.Errorf("append must target the section just added: %v", ops[1])
	}
	if v := mapField(ops[1], "value"); v["type"] != "blocks/gen_new" {
		t.Errorf("append value: %v", v)
	}
	inst := mapField(body, "instance")
	if inst["section_created"] != true || inst["target"] != sid+".blocks[0]" {
		t.Errorf("instance: %v", inst)
	}
}

// TestBlockEdit_SectionNameNamesTheContainer: --section-name is the container
// section's display name, stored as its settings.title. A container addressed
// by --target is renamed by a props merge on the section itself, whether the
// block is being created into it or updated in place.
func TestBlockEdit_SectionNameNamesTheContainer(t *testing.T) {
	content := writeTempLiquid(t, testGenSchema)
	cases := []struct {
		name string
		vals map[string]any
	}{
		{"create-into-existing-container", map[string]any{
			"session": "ose_x", "content": content, "template": "index", "target": "111.blocks", "section-name": "商品推荐"}},
		{"update-in-place", map[string]any{
			"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks[1]", "section-name": "商品推荐"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := newBlockServer(t)
			body, err := blockEditExec(t, bs, tc.vals)
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			ops := bs.operations(t)
			last := ops[len(ops)-1]
			if last["op"] != "replace_props" || last["target"] != "111" {
				t.Errorf("renaming an existing container is a props merge on the section: %v", ops)
			}
			if mapField(last, "props")["title"] != "商品推荐" {
				t.Errorf("the name goes to settings.title: %v", last)
			}
			if mapField(body, "instance")["section_name"] != "商品推荐" {
				t.Errorf("instance: %v", body["instance"])
			}
		})
	}
}

// TestBlockEdit_RefusedSectionNameKeepsThePlacement: a container whose schema
// has no title field refuses the rename. The block still landed, so the call
// must succeed — with the name dropped from the echo and the reason kept in
// applied — instead of sending the caller into placement recovery.
func TestBlockEdit_RefusedSectionNameKeepsThePlacement(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{1: "invalid_field:title"}
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index",
		"target": "111.blocks", "section-name": "商品推荐",
	})
	if err != nil {
		t.Fatalf("a refused name must not fail the placement: %v", err)
	}
	inst := mapField(body, "instance")
	if _, named := inst["section_name"]; named {
		t.Errorf("the name did not stick, so it must not be echoed: %v", inst)
	}
	if inst["target"] != "111.blocks[2]" {
		t.Errorf("the placement stands: %v", inst)
	}
	applied, _ := body["applied"].([]map[string]any)
	if len(applied) != 2 || applied[1]["result"] != "invalid_field:title" {
		t.Errorf("applied must carry why the rename failed: %v", applied)
	}
}

// TestBlockEdit_SectionNameStaysOutOfTheAddedSection: naming a container the
// CLI adds in this same batch is still its own trailing op — an add_section
// carrying a field the container's schema does not declare would fail the add,
// and with it the append that depends on it.
func TestBlockEdit_SectionNameStaysOutOfTheAddedSection(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "section-name": "商品推荐",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ops := bs.operations(t)
	if len(ops) != 3 {
		t.Fatalf("want add_section + append_array_item + rename, got %v", ops)
	}
	sid := getString(ops[0], "section_id")
	if s := mapField(mapField(ops[0], "value"), "settings"); len(s) != 0 {
		t.Errorf("the added section must stay an empty shell: %v", s)
	}
	if ops[2]["op"] != "replace_props" || ops[2]["target"] != sid {
		t.Errorf("the rename must address the section just added: %v", ops[2])
	}
	if mapField(ops[2], "props")["title"] != "商品推荐" {
		t.Errorf("props: %v", ops[2])
	}
	inst := mapField(body, "instance")
	if inst["section_created"] != true || inst["section_name"] != "商品推荐" {
		t.Errorf("instance: %v", inst)
	}
}

// TestBlockEdit_RefusedNameKeepsTheSectionItJustAdded: same non-fatal contract
// on the create path — the container and the block stay, only the name is gone.
func TestBlockEdit_RefusedNameKeepsTheSectionItJustAdded(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{2: "invalid_field:title"}
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "section-name": "商品推荐",
	})
	if err != nil {
		t.Fatalf("a refused name must not fail the placement: %v", err)
	}
	inst := mapField(body, "instance")
	if _, named := inst["section_name"]; named {
		t.Errorf("the name did not stick, so it must not be echoed: %v", inst)
	}
	if inst["section_created"] != true {
		t.Errorf("the container still went in: %v", inst)
	}
}

func TestBlockEdit_UpdateInPlaceMigratesSettings(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks[1]",
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	patches := bs.writesTo("/gen-blocks", http.MethodPatch)
	if len(patches) != 1 {
		t.Fatalf("patches: %v", patches)
	}
	sent := mapField(mapField(patches[0], "body"), "settings")
	if sent["type"] != "blocks/gen_aaa" || mapField(sent, "settings")["title"] != "cur" {
		t.Errorf("PATCH settings: %v", sent)
	}
	ops := bs.operations(t)
	if len(ops) != 1 || ops[0]["op"] != "replace_props" || ops[0]["target"] != "111.blocks.1" {
		t.Fatalf("ops: %v", ops)
	}
	props := mapField(ops[0], "props")
	if props["title"] != "cur" || props["subtitle"] != "Sub" {
		t.Errorf("migrated props: %v", props)
	}
	if _, stale := props["old_key"]; stale {
		t.Errorf("old_key must be dropped from the migrated settings: %v", props)
	}
	if body["branched"] != false || body["previous_type"] != nil || body["type"] != "blocks/gen_aaa" {
		t.Errorf("body: %v", body)
	}
	if inst := mapField(body, "instance"); inst["target"] != "111.blocks[1]" {
		t.Errorf("instance: %v", inst)
	}
}

// TestBlockEdit_UpdateBranchedRepointsInPlace: the fork keeps the instance
// where it is — update_slot swaps the type it points at (replace_props answers
// invalid_field:type) and the migrated settings follow in their own op, so
// nothing depends on an index shifting.
func TestBlockEdit_UpdateBranchedRepointsInPlace(t *testing.T) {
	bs := newBlockServer(t)
	bs.branched = true
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks.1",
		"ops": `{"subtitle":"z"}`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if batches := bs.opBatches(t); len(batches) != 1 {
		t.Fatalf("want one batch, got %v", batches)
	}
	ops := bs.operations(t)
	if len(ops) != 3 {
		t.Fatalf("ops: %v", ops)
	}
	if ops[0]["op"] != "update_slot" || ops[0]["target"] != "111.blocks.1" || mapField(ops[0], "props")["type"] != "blocks/gen_bbb" {
		t.Errorf("type swap: %v", ops[0])
	}
	for _, op := range ops {
		if op["op"] == "remove_array_item" || op["op"] == "move_array_item" {
			t.Errorf("the instance must not be rebuilt: %v", ops)
		}
	}
	vs := mapField(ops[1], "props")
	if ops[1]["op"] != "replace_props" || ops[1]["target"] != "111.blocks.1" || vs["title"] != "cur" || vs["subtitle"] != "Sub" {
		t.Errorf("migrated settings: %v", ops[1])
	}
	if _, stale := vs["old_key"]; stale {
		t.Errorf("migration must drop keys absent from the new schema: %v", vs)
	}
	if ops[2]["op"] != "replace_props" || ops[2]["target"] != "111.blocks.1" || mapField(ops[2], "props")["subtitle"] != "z" {
		t.Errorf("ops replace_props: %v", ops[2])
	}
	if body["branched"] != true || body["type"] != "blocks/gen_bbb" || body["previous_type"] != "blocks/gen_aaa" {
		t.Errorf("body: %v", body)
	}
}

// TestBlockEdit_BranchedRollbackPutsTheTypeBack: with the type swapped and the
// migration refused, the instance points at a block whose file the rollback is
// about to undo — the type goes back first so nothing dangles.
func TestBlockEdit_BranchedRollbackPutsTheTypeBack(t *testing.T) {
	bs := newBlockServer(t)
	bs.branched = true
	bs.failResults = map[int]string{1: "invalid_field:settings"} // the migration
	_, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks.1",
	})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want api error, got %v", err)
	}
	if env := exitErr.Envelope(); env["reverted"] != true {
		t.Errorf("envelope: %v", env)
	}
	undo := bs.opBatches(t)
	last := undo[len(undo)-1]
	if len(last) != 1 || last[0]["op"] != "update_slot" || mapField(last[0], "props")["type"] != "blocks/gen_aaa" {
		t.Errorf("want the type put back before the file is reverted, got %v", last)
	}
	reverts := bs.writesTo("/gen-blocks/revert", http.MethodPost)
	if len(reverts) != 1 {
		t.Errorf("want one revert, got %d", len(reverts))
	}
}

func TestBlockEdit_UpdateWithSettingsOverridesPageValues(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks[1]",
		"settings": `{"title":"given"}`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	sent := mapField(mapField(bs.writesTo("/gen-blocks", http.MethodPatch)[0], "body"), "settings")
	if mapField(sent, "settings")["title"] != "given" || sent["type"] != "blocks/gen_aaa" {
		t.Errorf("PATCH settings: %v", sent)
	}
	if props := mapField(bs.operations(t)[0], "props"); props["title"] != "given" {
		t.Errorf("props: %v", props)
	}
}

// TestBlockEdit_WithoutTemplateWritesOnly: with no --template the shortcut
// stops after the file write, on either path — no ops, no instance, no preview.
func TestBlockEdit_WithoutTemplateWritesOnly(t *testing.T) {
	content := writeTempLiquid(t, testGenSchema)

	t.Run("create", func(t *testing.T) {
		bs := newBlockServer(t)
		body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": content})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		if len(bs.writes) != 1 || bs.writes[0]["method"] != http.MethodPost {
			t.Errorf("want only the create call, got %v", bs.writes)
		}
		if body["instance"] != nil || body["preview_url"] != nil {
			t.Errorf("no placement expected: %v", body)
		}
		if mapField(body, "doc")["location"] != "gen_new.liquid" {
			t.Errorf("doc: %v", body["doc"])
		}
	})

	t.Run("update", func(t *testing.T) {
		bs := newBlockServer(t)
		body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "content": content})
		if err != nil {
			t.Fatalf("err: %v", err)
		}
		// No instance to read current values from, so the card keeps its defaults.
		sent := mapField(mapField(bs.writesTo("/gen-blocks", http.MethodPatch)[0], "body"), "settings")
		if len(sent) != 1 || sent["type"] != "blocks/gen_aaa" {
			t.Errorf("PATCH settings: %v", sent)
		}
		if body["settings_defaulted"] != true || body["instance"] != nil {
			t.Errorf("body: %v", body)
		}
	})
}

func TestBlockEdit_ValidationRefusesBeforeAnyRequest(t *testing.T) {
	bs := newBlockServer(t)
	content := writeTempLiquid(t, testGenSchema)
	cases := []struct {
		name  string
		vals  map[string]any
		wants string
	}{
		{"session", map[string]any{"content": content}, "--session"},
		{"content", map[string]any{"session": "ose_x"}, "--content"},
		{"target-without-template", map[string]any{"session": "ose_x", "content": content, "target": "111.blocks"}, "--target requires --template"},
		{"ops-without-target", map[string]any{"session": "ose_x", "content": content, "template": "index", "ops": `{"a":1}`}, "--ops requires"},
		{"settings-without-id", map[string]any{"session": "ose_x", "content": content, "settings": `{"a":1}`}, "--settings only applies"},
		{"two-stdin", map[string]any{"session": "ose_x", "id": "gen_aaa", "content": "-", "settings": "-"}, "stdin"},
		{"bad-id", map[string]any{"session": "ose_x", "id": "hero", "content": content}, "invalid block id"},
		{"create-with-instance-target", map[string]any{"session": "ose_x", "content": content, "template": "index", "target": "111.blocks[2]"}, "container path"},
		{"update-with-container-target", map[string]any{"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks"}, "instance path"},
		{"settings-type-mismatch", map[string]any{"session": "ose_x", "id": "gen_aaa", "content": content, "settings": `{"type":"blocks/gen_zzz"}`}, "does not match"},
		{"ops-array", map[string]any{"session": "ose_x", "content": content, "template": "index", "target": "111.blocks", "ops": `[{"op":"x"}]`}, "JSON object"},
		{"section-name-without-template", map[string]any{"session": "ose_x", "content": content, "section-name": "商品推荐"}, "--section-name requires --template"},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			_, err := blockEditExec(t, bs, tc.vals)
			wantValidation(t, err, tc.wants)
		})
	}
	if len(bs.writes) != 0 {
		t.Errorf("validation failures must not send anything, got %v", bs.writes)
	}
}

func TestBlockEdit_PageChecksRunBeforeTheWrite(t *testing.T) {
	bs := newBlockServer(t)
	content := writeTempLiquid(t, testGenSchema)
	for name, vals := range map[string]map[string]any{
		"section-missing":     {"session": "ose_x", "content": content, "template": "index", "target": "999.blocks"},
		"index-out-of-range":  {"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks[7]"},
		"type-mismatch":       {"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks[0]"},
		"nested-not-a-parent": {"session": "ose_x", "content": content, "template": "index", "target": "111.blocks[0].blocks[3].blocks"},
	} {
		t.Run(name, func(t *testing.T) {
			_, err := blockEditExec(t, bs, vals)
			wantValidation(t, err, "")
		})
	}
	if n := len(bs.writesTo("/gen-blocks", http.MethodPost)) + len(bs.writesTo("/gen-blocks", http.MethodPatch)); n != 0 {
		t.Errorf("page checks must run before the file write, got %d writes", n)
	}
}

func TestBlockEdit_InvalidLiquidIsAnAPIErrorAtWriteStage(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, "BAD"), "template": "index", "target": "111.blocks"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("want api error, got %v", err)
	}
	env := exitErr.Envelope()
	if env["stage"] != "write" || env["oseid"] != "ose_x" || !strings.Contains(err.Error(), "json_parse_error") {
		t.Errorf("envelope: %v", env)
	}
	if len(bs.writesTo("/operations", http.MethodPost)) != 0 {
		t.Errorf("no placement after a failed write")
	}
}

// TestBlockEdit_PlacementFailureRollsBackTheWrite: an op that had to land and
// did not leaves nothing behind — the file write goes back through the server
// snapshot, so the caller can send the very same command again.
func TestBlockEdit_PlacementFailureRollsBackTheWrite(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{0: "target_not_found"}
	_, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("want api error, got %v", err)
	}
	env := exitErr.Envelope()
	if env["stage"] != "place" || env["block_type"] != "blocks/gen_new" || env["reverted"] != true {
		t.Errorf("envelope: %v", env)
	}
	if _, kept := env["revert_id"]; kept {
		t.Errorf("a rolled-back write has nothing left to revert: %v", env)
	}
	if failed, _ := env["failed"].([]string); len(failed) != 1 || failed[0] != "append" {
		t.Errorf("failed: %v", env["failed"])
	}
	if !strings.Contains(getString(env, "hint"), "same command again") {
		t.Errorf("hint: %v", env["hint"])
	}
	if n := len(bs.writesTo("/gen-blocks/revert", http.MethodPost)); n != 1 {
		t.Errorf("want one revert, got %d", n)
	}
}

// TestBlockEdit_RollbackAlsoDropsTheAddedContainer: the container the CLI adds
// itself is part of what this call wrote, so it goes too — otherwise a retry
// stacks empty "_blocks" shells on the page.
func TestBlockEdit_RollbackAlsoDropsTheAddedContainer(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{1: "block_type_invalid"} // the append, after add_section
	_, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want api error, got %v", err)
	}
	env := exitErr.Envelope()
	if env["reverted"] != true || !strings.HasSuffix(getString(env, "container"), ".blocks") {
		t.Errorf("envelope: %v", env)
	}
	last := bs.opBatches(t)
	removal := last[len(last)-1]
	if len(removal) != 1 || removal[0]["op"] != "remove_section" || removal[0]["target"] == "" {
		t.Errorf("want the added container removed, got %v", removal)
	}
}

// TestBlockEdit_RollbackFailureTellsTheCallerNotToRetry: with the file still in
// the session, sending the command again would write a second one.
func TestBlockEdit_RollbackFailureTellsTheCallerNotToRetry(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{0: "target_not_found"}
	bs.revertFails = true
	_, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) {
		t.Fatalf("want api error, got %v", err)
	}
	env := exitErr.Envelope()
	if env["revert_failed"] != true || env["revert_id"] != "rev_create" || getString(env, "revert_error") == "" {
		t.Errorf("envelope: %v", env)
	}
	if h := getString(env, "hint"); !strings.Contains(h, "instead of sending the command again") {
		t.Errorf("hint: %v", h)
	}
}

// TestBlockEdit_BranchedAppendFailureStopsBeforeTheRemove: the old instance is
// the merchant's only working card until the new one is in.
func TestBlockEdit_BranchedAppendFailureStopsBeforeTheRemove(t *testing.T) {
	bs := newBlockServer(t)
	bs.branched = true
	bs.failResults = map[int]string{0: "block_type_invalid"}
	_, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks[1]",
	})
	if err == nil {
		t.Fatal("want an api error")
	}
	for _, batch := range bs.opBatches(t) {
		for _, op := range batch {
			if op["op"] == "remove_array_item" {
				t.Errorf("the old instance must stay: %v", batch)
			}
		}
	}
	if n := len(bs.writesTo("/gen-blocks/revert", http.MethodPost)); n != 1 {
		t.Errorf("want the branched file reverted, got %d reverts", n)
	}
}

// TestBlockEdit_DegradedOpsKeepThePlacement: the block landed, so the call
// succeeds and names what did not stick instead of sending the caller into a
// retry that would write the file twice.
func TestBlockEdit_DegradedOpsKeepThePlacement(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{1: "invalid_field:subtitle"} // --ops, after the migration
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks[1]",
		"ops": `{"subtitle":"z"}`,
	})
	if err != nil {
		t.Fatalf("a degraded op must not fail the call: %v", err)
	}
	degraded, _ := body["degraded"].([]string)
	if len(degraded) != 1 || degraded[0] != "ops" {
		t.Errorf("degraded: %v", body["degraded"])
	}
	if n := len(bs.writesTo("/gen-blocks/revert", http.MethodPost)); n != 0 {
		t.Errorf("the placement stands, nothing to revert (%d reverts)", n)
	}
}

func TestBlockEdit_DryRunSendsNothing(t *testing.T) {
	content := writeTempLiquid(t, "{% schema %}{}{% endschema %}")
	for name, vals := range map[string]map[string]any{
		"create": {"session": "ose_x", "content": content, "template": "index", "target": "111.blocks", "ops": `{"title":"x"}`},
		// no --target: the CLI adds the container itself, so the batch is
		// add_section + append_array_item (the path with no golden before).
		"new-section": {"session": "ose_x", "content": content, "template": "index"},
		"edit":        {"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks.1", "theme": "t1"},
		"file":        {"session": "ose_x", "content": content},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := blockEditExecute(context.Background(), common.ExecInput{DryRun: true, Flags: shortcutFlags(t, blockEditShortcut, vals)})
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			if len(res.Plans) == 0 || res.Body != nil {
				t.Fatalf("dry-run must return plans only: %+v", res)
			}
			snapshot(t, "block_edit_dry_run_"+name, plansToMap(res.Plans))
		})
	}
}

// ─────────── +get ───────────

func TestBlockGet_ListsPlacements(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "blocks/gen_aaa"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if body["type"] != "blocks/gen_aaa" || body["ref_count"] != 3 || body["saved"] != true {
		t.Errorf("body: %v", body)
	}
	rows, _ := body["instances"].([]map[string]any)
	if len(rows) != 3 || rows[0]["template"] != "index" || rows[0]["target"] != "111.blocks[1]" || rows[2]["template"] != "product" || rows[2]["target"] != "333.blocks[0]" {
		t.Errorf("instances: %v", rows)
	}
	if body["instance"] != nil || body["name"] != nil {
		t.Errorf("no instance/name without --section/--with-content: %v", body)
	}
	gets := bs.writesTo("/gen-blocks", http.MethodGet)
	if len(bs.writes) != 1 || mapField(gets[0], "body")["with_content"] != "" {
		t.Errorf("want one GET without with_content: %v", bs.writes)
	}
}

func TestBlockGet_WithContentAddsSourceAndName(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "with-content": true})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	if !strings.Contains(getString(mapField(body, "doc"), "content"), "{% schema %}") {
		t.Errorf("doc.content missing: %v", body["doc"])
	}
	if name := asMap(body["name"]); name["zh-CN"] != "卡片" {
		t.Errorf("name: %v", body["name"])
	}
	if mapField(bs.writes[0], "body")["with_content"] != "true" {
		t.Errorf("with_content query missing: %v", bs.writes[0])
	}
}

func TestBlockGet_SectionReturnsInstanceWithSettings(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "section": "111"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	inst := asMap(body["instance"])
	if inst["template"] != "index" || inst["target"] != "111.blocks[1]" || asMap(inst["settings"])["title"] != "cur" {
		t.Errorf("instance: %v", body["instance"])
	}
	if _, listed := body["instances"]; listed {
		t.Errorf("--section must not also list every placement")
	}
}

func TestBlockGet_UnknownIdPassesThrough(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_nope"})
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 passthrough, got %v", err)
	}
}

func TestBlockGet_Refusals(t *testing.T) {
	cases := []struct {
		name      string
		vals      map[string]any
		wants     string
		preflight bool // refused before any request
	}{
		{"no session", map[string]any{"id": "gen_aaa"}, "--session", true},
		{"bad id", map[string]any{"session": "ose_x", "id": "slide"}, "invalid block id", true},
		{"section is not a placement", map[string]any{"session": "ose_x", "id": "gen_aaa", "section": "999"}, "no recorded instance", false},
		{"section without the block", map[string]any{"session": "ose_x", "id": "gen_aaa", "section": "222", "template": "index"}, "has no block of type", false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			bs := newBlockServer(t)
			_, err := blockGetExec(t, bs, tc.vals)
			wantValidation(t, err, tc.wants)
			if tc.preflight && len(bs.writes) != 0 {
				t.Errorf("no requests expected: %v", bs.writes)
			}
		})
	}
}

func TestBlockGet_DryRun(t *testing.T) {
	for name, vals := range map[string]map[string]any{
		"list":    {"session": "ose_x", "id": "gen_aaa", "with-content": true},
		"section": {"session": "ose_x", "id": "blocks/gen_aaa", "section": "111.blocks.1"},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := blockGetExecute(context.Background(), common.ExecInput{DryRun: true, Flags: shortcutFlags(t, blockGetShortcut, vals)})
			if err != nil {
				t.Fatalf("err: %v", err)
			}
			snapshot(t, "block_get_dry_run_"+name, plansToMap(res.Plans))
		})
	}
}

// ─────────── helpers ───────────

func TestNormalizeBlockTarget(t *testing.T) {
	for in, want := range map[string]string{
		"111.blocks.0":          "111.blocks[0]",
		"111.blocks.2.blocks.1": "111.blocks[2].blocks[1]",
		"111.blocks[3]":         "111.blocks[3]",
		"111.blocks":            "111.blocks",
		"1750398354466":         "1750398354466",
	} {
		if got := normalizeBlockTarget(in); got != want {
			t.Errorf("%s → %s, want %s", in, got, want)
		}
	}
}

func TestParseGenInstances(t *testing.T) {
	rows := parseGenInstances([]any{
		map[string]any{"index": []any{"1.blocks.0", "2.blocks.3"}},
		map[string]any{"product": []any{"3.blocks.1.blocks.0"}},
		"garbage",
	})
	if len(rows) != 3 || rows[2].Template != "product" || rows[2].Target != "3.blocks[1].blocks[0]" {
		t.Errorf("rows: %+v", rows)
	}
	// object shape: one map keyed by template
	rows = parseGenInstances(map[string]any{"index": []any{"1.blocks.0"}, "product": []any{"3.blocks.1"}})
	if len(rows) != 2 {
		t.Errorf("object-shaped instances: %+v", rows)
	}
	if rows := parseGenInstances(nil); len(rows) != 0 {
		t.Errorf("nil instances: %+v", rows)
	}
}

func TestMigrateSettings(t *testing.T) {
	got := migrateSettings(map[string]any{"a": 1, "b": 2}, map[string]bool{"a": true, "b": true}, map[string]any{"a": 9, "zombie": 0})
	if got["a"] != 9 || got["b"] != 2 {
		t.Errorf("got %v", got)
	}
	if _, ok := got["zombie"]; ok {
		t.Errorf("dropped keys must not survive: %v", got)
	}
}

// TestHelp_BlockCommands checks each +command's help resolves through its own
// Service path: the page-editing +edit stays under themes, the block-editing
// one under themes block, and neither shadows the other.
func TestHelp_BlockCommands(t *testing.T) {
	cases := []struct {
		path   []string
		want   []string
		absent string // must not appear among the flags
	}{
		{[]string{"themes", "block", "+edit"},
			[]string{"+edit", "--session", "--content", "--id", "--template", "--target", "--settings", "--ops", "--section-name", "branched", "reverted", "degraded"},
			"--promote"}, // saving is themes +edit's job
		{[]string{"themes", "block", "+get"},
			[]string{"+get", "--session", "--id", "--section", "--template", "--with-content", "ref_count"}, ""},
		{[]string{"themes", "+edit"}, []string{"--ops", "--promote"}, "--content"},
	}
	for _, c := range cases {
		t.Run(strings.Join(c.path, " "), func(t *testing.T) {
			out := helpFor(t, c.path...)
			for _, want := range c.want {
				if !strings.Contains(out, want) {
					t.Errorf("help missing %q in:\n%s", want, out)
				}
			}
			if flags := flagsSection(out); c.absent != "" && strings.Contains(flags, c.absent) {
				t.Errorf("help must not expose %s:\n%s", c.absent, flags)
			}
		})
	}
}
