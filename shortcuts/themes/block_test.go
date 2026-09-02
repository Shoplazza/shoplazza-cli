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

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const testGenSchema = `{% schema %}
{"name":"card","settings":[{"id":"title","type":"text","default":"Hello"},{"id":"subtitle","type":"text","default":"Sub"}],
 "presets":[{"name":"card","cname":{"en-US":"Card","zh-CN":"卡片"},"settings":{"title":"Hello","subtitle":"Sub"}}]}
{% endschema %}`

func blockFlags(t *testing.T, sc common.Shortcut, vals map[string]any) common.FlagSet {
	t.Helper()
	cmd := &cobra.Command{Use: sc.Command}
	for _, f := range sc.Flags {
		if f.Type == common.FlagBool {
			cmd.Flags().Bool(f.Name, false, "")
		} else {
			cmd.Flags().String(f.Name, "", "")
		}
	}
	for k, v := range vals {
		if err := cmd.Flags().Set(k, fmt.Sprint(v)); err != nil {
			t.Fatalf("set flag %s: %v", k, err)
		}
	}
	return common.NewCobraFlagSet(cmd)
}

// blockServer fakes the gen-blocks family plus the page endpoints the two
// shortcuts orchestrate, recording every write in order.
type blockServer struct {
	srv *httptest.Server

	mu          sync.Mutex
	writes      []map[string]any
	failResults map[int]string
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
			for i, o := range ops {
				om, _ := o.(map[string]any)
				res := "success"
				if fr, ok := bs.failResults[i]; ok {
					res = fr
				} else if om["op"] == "add_section" {
					bs.added = append(bs.added, fmt.Sprintf("sec_new%d", len(bs.added)+1))
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

func (bs *blockServer) operations(t *testing.T) []map[string]any {
	t.Helper()
	ws := bs.writesTo("/operations", http.MethodPost)
	if len(ws) != 1 {
		t.Fatalf("want exactly one operations batch, got %d", len(ws))
	}
	return mapSlice(mapField(ws[0], "body")["operations"])
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
	res, err := blockEditExecute(context.Background(), common.ExecInput{Flags: blockFlags(t, blockEditShortcut, vals), Tool: "edit", Client: client.New(bs.srv.URL)})
	return res.Body, err
}

func blockGetExec(t *testing.T, bs *blockServer, vals map[string]any) (map[string]any, error) {
	t.Helper()
	res, err := blockGetExecute(context.Background(), common.ExecInput{Flags: blockFlags(t, blockGetShortcut, vals), Tool: "get", Client: client.New(bs.srv.URL)})
	return res.Body, err
}

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

func TestBlockEdit_CreateWithoutTemplateWritesOnly(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema)})
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
}

func TestBlockEdit_CreateWithoutTargetWrapsInBlocksSection(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index"})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ops := bs.operations(t)
	if len(ops) != 1 || ops[0]["op"] != "add_section" {
		t.Fatalf("ops: %v", ops)
	}
	value := mapField(ops[0], "value")
	blocks := mapSlice(value["blocks"])
	if value["type"] != "_blocks" || len(blocks) != 1 || blocks[0]["type"] != "blocks/gen_new" {
		t.Errorf("add_section value: %v", value)
	}
	inst := mapField(body, "instance")
	if inst["section_created"] != true || inst["target"] != "sec_new1.blocks[0]" {
		t.Errorf("instance: %v", inst)
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

func TestBlockEdit_UpdateBranchedSwapsInstanceType(t *testing.T) {
	bs := newBlockServer(t)
	bs.branched = true
	body, err := blockEditExec(t, bs, map[string]any{
		"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks.1",
		"ops": `{"subtitle":"z"}`,
	})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	ops := bs.operations(t)
	// remove old + append new (migrated settings), then --ops. The target is the
	// container's last index (1 of 2), so the move-back is an identity and skipped.
	if len(ops) != 3 {
		t.Fatalf("ops: %v", ops)
	}
	if ops[0]["op"] != "remove_array_item" || ops[0]["target"] != "111.blocks.1" {
		t.Errorf("remove: %v", ops[0])
	}
	if ops[1]["op"] != "append_array_item" || ops[1]["target"] != "111.blocks" {
		t.Errorf("append: %v", ops[1])
	}
	val := mapField(ops[1], "value")
	vs := mapField(val, "settings")
	if val["type"] != "blocks/gen_bbb" || vs["title"] != "cur" || vs["subtitle"] != "Sub" {
		t.Errorf("append value not migrated to new card: %v", val)
	}
	if _, stale := vs["old_key"]; stale {
		t.Errorf("append value must drop keys absent from the new schema: %v", vs)
	}
	if ops[2]["op"] != "replace_props" || ops[2]["target"] != "111.blocks.1" || mapField(ops[2], "props")["subtitle"] != "z" {
		t.Errorf("ops replace_props: %v", ops[2])
	}
	if body["branched"] != true || body["type"] != "blocks/gen_bbb" || body["previous_type"] != "blocks/gen_aaa" {
		t.Errorf("body: %v", body)
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

func TestBlockEdit_UpdateWithoutTemplateDefaultsSettings(t *testing.T) {
	bs := newBlockServer(t)
	body, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "content": writeTempLiquid(t, testGenSchema)})
	if err != nil {
		t.Fatalf("err: %v", err)
	}
	sent := mapField(mapField(bs.writesTo("/gen-blocks", http.MethodPatch)[0], "body"), "settings")
	if len(sent) != 1 || sent["type"] != "blocks/gen_aaa" {
		t.Errorf("PATCH settings: %v", sent)
	}
	if body["settings_defaulted"] != true || body["instance"] != nil {
		t.Errorf("body: %v", body)
	}
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
			var exitErr *output.ExitError
			if !errors.As(err, &exitErr) || exitErr.Code != output.ExitValidation {
				t.Fatalf("want validation error, got %v", err)
			}
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

func TestBlockEdit_PlacementFailureKeepsTheWrite(t *testing.T) {
	bs := newBlockServer(t)
	bs.failResults = map[int]string{0: "target_not_found"}
	_, err := blockEditExec(t, bs, map[string]any{"session": "ose_x", "content": writeTempLiquid(t, testGenSchema), "template": "index", "target": "111.blocks"})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("want api error, got %v", err)
	}
	env := exitErr.Envelope()
	if env["stage"] != "place" || env["block_type"] != "blocks/gen_new" || env["revert_id"] != "rev_create" {
		t.Errorf("envelope: %v", env)
	}
	if failed, _ := env["failed"].([]int); len(failed) != 1 || failed[0] != 0 {
		t.Errorf("failed: %v", env["failed"])
	}
	if !strings.Contains(getString(env, "hint"), "revert-gen") || !strings.Contains(getString(env, "hint"), "--id gen_new") {
		t.Errorf("hint: %v", env["hint"])
	}
}

func TestBlockEdit_DryRunSendsNothing(t *testing.T) {
	content := writeTempLiquid(t, "{% schema %}{}{% endschema %}")
	for name, vals := range map[string]map[string]any{
		"create": {"session": "ose_x", "content": content, "template": "index", "target": "111.blocks", "ops": `{"title":"x"}`},
		"edit":   {"session": "ose_x", "id": "gen_aaa", "content": content, "template": "index", "target": "111.blocks.1", "theme": "t1"},
		"file":   {"session": "ose_x", "content": content},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := blockEditExecute(context.Background(), common.ExecInput{DryRun: true, Flags: blockFlags(t, blockEditShortcut, vals)})
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

func TestBlockGet_SectionNeedsTemplateWhenUnrecorded(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "section": "999"})
	wantValidation(t, err, "no recorded instance")
	_, err = blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_aaa", "section": "222", "template": "index"})
	wantValidation(t, err, "has no block of type")
}

func TestBlockGet_UnknownIdPassesThrough(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "gen_nope"})
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != http.StatusNotFound {
		t.Fatalf("want 404 passthrough, got %v", err)
	}
}

func TestBlockGet_Validation(t *testing.T) {
	bs := newBlockServer(t)
	_, err := blockGetExec(t, bs, map[string]any{"id": "gen_aaa"})
	wantValidation(t, err, "--session")
	_, err = blockGetExec(t, bs, map[string]any{"session": "ose_x", "id": "slide"})
	wantValidation(t, err, "invalid block id")
	if len(bs.writes) != 0 {
		t.Errorf("no requests expected: %v", bs.writes)
	}
}

func TestBlockGet_DryRun(t *testing.T) {
	for name, vals := range map[string]map[string]any{
		"list":    {"session": "ose_x", "id": "gen_aaa", "with-content": true},
		"section": {"session": "ose_x", "id": "blocks/gen_aaa", "section": "111.blocks.1"},
	} {
		t.Run(name, func(t *testing.T) {
			res, err := blockGetExecute(context.Background(), common.ExecInput{DryRun: true, Flags: blockFlags(t, blockGetShortcut, vals)})
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
