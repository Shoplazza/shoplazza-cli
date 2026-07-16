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

	mu       sync.Mutex
	writes   []map[string]any
	failPath string // first write whose path contains this substring fails with 500
	conflict bool   // promote responds conflict=true
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
			_ = json.NewEncoder(w).Encode(map[string]any{"templates": []any{map[string]any{"id": "d_index", "location": "index.liquid"}}})
		case strings.HasSuffix(p, "/edit-sessions") && r.Method == http.MethodPost:
			_ = json.NewEncoder(w).Encode(map[string]any{"oseid": "ose_new"})
		case strings.HasSuffix(p, "/sections") && r.Method == http.MethodGet:
			_ = json.NewEncoder(w).Encode(map[string]any{"data": map[string]any{
				"schemas": map[string]any{
					"hero_slideshow": map[string]any{
						"max_blocks": 2,
						"blocks":     []any{map[string]any{"type": "slide", "name": map[string]any{"zh-CN": "幻灯"}}},
					},
				},
				"sections": map[string]any{
					"page_sections": []any{
						map[string]any{"id": 111, "type": "hero_slideshow", "display": true, "settings": map[string]any{},
							"blocks": []any{map[string]any{"type": "slide", "settings": map[string]any{}}}},
						map[string]any{"id": 222, "type": "shoplazza://apps/page-builder/blocks/custom-9527", "display": true,
							"settings": map[string]any{}, "blocks": []any{}},
					},
					// dedicated header group (real df423620 shape — numeric ids, not the
					// semantic-id fixed group). Exercises sectionsByArea's group path.
					"header_sections": []any{
						map[string]any{"id": "hsec1", "type": "header_bar", "display": true, "settings": map[string]any{}, "blocks": []any{}},
					},
					"sections": []any{}, // truly-fixed cards (cart_drawer etc.)
				},
			}})
		case strings.HasSuffix(p, "/promote"):
			if es.conflict { // real behavior: HTTP 409, not a {conflict:true} body
				w.WriteHeader(http.StatusConflict)
				_, _ = w.Write([]byte(`{"message":"edit session has conflict with draft, retry with force=true to overwrite"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"promoted": true})
		default: // write endpoints
			var body map[string]any
			_ = json.NewDecoder(r.Body).Decode(&body)
			es.mu.Lock()
			es.writes = append(es.writes, map[string]any{"method": r.Method, "path": p, "body": body})
			fail := es.failPath != "" && strings.Contains(p, es.failPath)
			if fail {
				es.failPath = "" // fail once
			}
			es.mu.Unlock()
			if fail {
				w.WriteHeader(http.StatusInternalServerError)
				_, _ = w.Write([]byte(`{"message":"boom"}`))
				return
			}
			_ = json.NewEncoder(w).Encode(map[string]any{"section_id": "sec_new", "html": "<div/>"})
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
	if len(applied) != 2 {
		t.Fatalf("applied = %v", applied)
	}
	preview := body["preview_url"].(string)
	if !strings.Contains(preview, "unit.myshoplaza.com") || !strings.Contains(preview, "oseid=ose_new") {
		t.Errorf("preview_url = %q", preview)
	}
	// update_slot must carry block coordinates parsed from the target.
	slot := es.writes[1]
	if !strings.HasSuffix(slot["path"].(string), "/sections/111/slot") {
		t.Errorf("slot path = %v", slot["path"])
	}
	sbody := slot["body"].(map[string]any)
	if fmt.Sprint(sbody["block_index"]) != "0" || sbody["doc_id"] != "d_index" || sbody["theme_id"] != "t_pub" {
		t.Errorf("slot body = %v", sbody)
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

func TestEdit_MixedBatchRoutesPbAndBackfillsBody(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	body, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"b"}},
		         {"op":"update_pb","target":"222","ops":[{"action":"update","targetId":"0","settings":{}}]}]`})
	if err != nil {
		t.Fatalf("editExecute: %v", err)
	}
	if len(body["applied"].([]map[string]any)) != 2 {
		t.Fatalf("applied = %v", body["applied"])
	}
	var pb map[string]any
	for _, wr := range es.writes {
		if strings.Contains(wr["path"].(string), "page-builder/blocks") {
			pb = wr["body"].(map[string]any)
		}
	}
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
	if applied[0]["new_section_id"] != "sec_new" {
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

func TestEdit_SectionPositionAndArea(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()

	// move a header_sections card: CLI reverse-looks-up area="header" from the
	// header_sections group; position "first" -> to_index 0.
	if _, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"move_section","target":"hsec1","position":"first"}]`}); err != nil {
		t.Fatalf("move header: %v", err)
	}
	mv := editWriteBody(es, http.MethodPatch, "/move")
	if mv == nil {
		t.Fatal("move-section never sent")
	}
	if mv["area"] != "header" || fmt.Sprint(mv["to_index"]) != "0" {
		t.Errorf("move body = %v, want area=header to_index=0", mv)
	}

	// add_section after page card 111 (page index 0) → to_index 1, no area (page flow).
	es.mu.Lock()
	es.writes = nil
	es.mu.Unlock()
	if _, err := editExec(t, es, map[string]any{"template": "index",
		"ops": `[{"op":"add_section","name":"rich_text","position":"after:111"}]`}); err != nil {
		t.Fatalf("add after: %v", err)
	}
	add := editWriteBody(es, http.MethodPost, "/sections")
	if add == nil {
		t.Fatal("add-section never sent")
	}
	if fmt.Sprint(add["to_index"]) != "1" {
		t.Errorf("add to_index = %v, want 1", add["to_index"])
	}
	if _, ok := add["area"]; ok {
		t.Errorf("page-flow add must not carry area: %v", add)
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

func TestEdit_FailFastPartialEnvelope(t *testing.T) {
	es := newEditServer(t)
	defer es.srv.Close()
	es.failPath = "/sections/222/props"

	_, err := editExec(t, es, map[string]any{"template": "index", "session": "ose_x",
		"ops": `[{"op":"replace_props","target":"111","props":{"a":"1"}},
		         {"op":"replace_props","target":"222","props":{"b":"2"}},
		         {"op":"replace_props","target":"111","props":{"c":"3"}}]`})
	var exitErr *output.ExitError
	if !errors.As(err, &exitErr) || exitErr.Code != output.ExitAPI {
		t.Fatalf("err = %v, want api ExitError", err)
	}
	env := exitErr.Envelope()
	if env["partial"] != true || env["oseid"] != "ose_x" {
		t.Errorf("envelope discriminators = %v", env)
	}
	if len(env["applied"].([]map[string]any)) != 1 {
		t.Errorf("applied = %v", env["applied"])
	}
	failed := env["failed"].(map[string]any)
	if failed["index"] != 1 || failed["target"] != "222" {
		t.Errorf("failed = %v", failed)
	}
	if fmt.Sprint(env["remaining"]) != "[2]" {
		t.Errorf("remaining = %v", env["remaining"])
	}
	if !strings.Contains(fmt.Sprint(env["hint"]), "--session ose_x") {
		t.Errorf("hint = %v", env["hint"])
	}
	// op #3 must never have been sent.
	for _, wr := range es.writes {
		b, _ := json.Marshal(wr["body"])
		if strings.Contains(string(b), `"c":"3"`) {
			t.Error("fail-fast violated: op after the failure was sent")
		}
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
	for _, want := range []string{"+edit", "--ops", "--session", "--promote", "fail-fast", "update_pb"} {
		if !strings.Contains(out, want) {
			t.Errorf("+edit help missing %q:\n%s", want, out)
		}
	}
}
