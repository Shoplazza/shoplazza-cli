package themes

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// themes +page — one-shot page read for agent-driven theme editing
// (docs/theme-page-edit-shortcuts.md §3, docs/plans/theme-page-edit/02-themes-page.md).
//
// Session semantics: omitting --session creates a fresh edit session (an edit
// draft copied from the theme draft) and echoes its oseid so the follow-up
// `themes +edit --session <oseid>` writes into the same snapshot.

// pagePlaceholder values stand in for runtime-resolved ids in --dry-run plans
// (strict zero-call semantics, docs/theme-page-edit-shortcuts.md §3.6).
const (
	phThemeID  = "<theme_id>"
	phDocID    = "<doc_id>"
	phOseid    = "<oseid>"
	phCustomID = "<custom_id>"
)

// standardPageTitles maps standard template names to their display titles for
// `+page --list` (custom templates carry their own title from the API).
var standardPageTitles = map[string]string{
	"index":      "首页",
	"product":    "商品详情",
	"collection": "商品分类",
	"cart":       "购物车",
	"page":       "自定义页面",
	"search":     "搜索",
}

var pageShortcut = common.Shortcut{
	Service: "themes",
	Command: "+page",
	Use:     "+page",
	Short:   "Read a template page: sections in render order, flattened blocks with ready-to-copy targets",
	Long: `Read one template page of a theme in a single call: sections in render
order with current settings, plus a depth-first flattened block list where
every row carries a pre-built "target" path — copy it verbatim into the ops
of "themes +edit".

Omitting --session creates a fresh edit session (an edit draft copied from
the theme draft) and echoes its oseid; pass that oseid to the follow-up
"themes +edit --session" so read and write share one snapshot. Pass --session
to re-read an existing edit session instead.

Extras: --include schema adds a compact zh-CN field-schema projection;
--include pb expands page-builder custom cards with their canvas text;
--area defaults to "all" (every area, each section tagged with its area);
pass page/header/footer/global to focus one; --list discovers the available
templates when the template name is ambiguous.`,
	Flags: []common.Flag{
		{Name: "template", Type: common.FlagString, Description: "Template name, e.g. index / product. Mutually exclusive with --file."},
		{Name: "file", Type: common.FlagString, Description: "Theme file path, e.g. templates/index.liquid. Mutually exclusive with --template."},
		{Name: "theme", Type: common.FlagString, Description: "Theme ID. Defaults to the published theme."},
		{Name: "session", Type: common.FlagString, Description: "Edit session id (oseid) to read. Omit to create a fresh session (echoed in the response)."},
		{Name: "area", Type: common.FlagString, Default: "all", Description: "Card area to read: all (default) | page | header | footer | global.", Completions: []string{"all", "page", "header", "footer", "global"}},
		{Name: "section", Type: common.FlagString, Description: "Focus on a single section id (a page-builder card auto-expands its canvas)."},
		{Name: "include", Type: common.FlagString, Description: "Comma-separated extras: schema (zh-CN field projection), pb (page-builder canvas)."},
		{Name: "list", Type: common.FlagBool, Description: "List available templates (standard + custom) instead of reading a page."},
	},
	Execute: pageExecute,
}

type pageInclude struct {
	Schema bool
	Pb     bool
}

func parseInclude(raw string) (pageInclude, error) {
	inc := pageInclude{}
	if raw == "" {
		return inc, nil
	}
	for _, part := range strings.Split(raw, ",") {
		switch strings.TrimSpace(part) {
		case "", "structure", "values": // structure+values are always included
		case "schema":
			inc.Schema = true
		case "pb":
			inc.Pb = true
		default:
			return inc, output.ErrValidation("unknown --include value %q", strings.TrimSpace(part)).
				WithHint("valid values: structure, values, schema, pb (comma-separated)")
		}
	}
	return inc, nil
}

func pageExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	themeID := in.Flags.GetString("theme")
	template := in.Flags.GetString("template")
	file := in.Flags.GetString("file")
	session := in.Flags.GetString("session")
	area := in.Flags.GetString("area")
	section := in.Flags.GetString("section")
	list := in.Flags.GetBool("list")

	inc, err := parseInclude(in.Flags.GetString("include"))
	if err != nil {
		return common.ExecResult{}, err
	}

	if list {
		if template != "" || file != "" || section != "" || inc.Schema || inc.Pb || area != "all" {
			return common.ExecResult{}, output.ErrValidation("--list cannot be combined with --template/--file/--section/--include/--area")
		}
		return pageList(ctx, in, themeID)
	}

	switch area {
	case "page", "header", "footer", "global", "all":
	default:
		return common.ExecResult{}, output.ErrValidation("invalid --area %q", area).
			WithHint("valid values: page, header, footer, global, all")
	}
	if _, _, err := templateLocation(template, file); err != nil {
		return common.ExecResult{}, err
	}

	if in.DryRun {
		return common.ExecResult{Plans: pageDryRunPlans(themeID, session, inc)}, nil
	}

	// Resolve chain + session (no --session means create one and echo it).
	themeID, docID, err := resolveThemeAndDoc(ctx, in.Client, themeID, template, file)
	if err != nil {
		return common.ExecResult{}, err
	}
	oseid, created := session, false
	if oseid == "" {
		resp, err := common.Send(ctx, in.Client, PlanCreateSession(themeID))
		if err != nil {
			return common.ExecResult{}, err
		}
		if oseid = extractOseid(resp); oseid == "" {
			return common.ExecResult{}, output.ErrInternal("create-session returned no oseid")
		}
		created = true
	}

	inner, err := fetchSections(ctx, in.Client, oseid, docID)
	if err != nil {
		if session != "" && isSessionNotFound(err) {
			return common.ExecResult{}, err // pass through verbatim, never auto-recreate
		}
		return common.ExecResult{}, err
	}
	byArea := sectionsByArea(inner)
	rowsByArea := map[string][]map[string]any{}
	for _, a := range []string{"page", "header", "footer", "global"} {
		rows := make([]map[string]any, 0, len(byArea[a]))
		for _, m := range byArea[a] {
			rows = append(rows, buildSectionRow(m))
		}
		rowsByArea[a] = rows
	}

	body := map[string]any{"oseid": oseid, "session_created": created}
	var selected []map[string]any
	switch area {
	case "page":
		selected = rowsByArea["page"]
		body["sections"] = selected
		body["areas"] = map[string]any{
			"header": len(rowsByArea["header"]), "footer": len(rowsByArea["footer"]), "global": len(rowsByArea["global"]),
		}
	case "header", "footer", "global":
		selected = rowsByArea[area]
		body["sections"] = selected
	case "all":
		// flat sections across areas, each tagged with its area, plus area counts
		for _, a := range []string{"page", "header", "footer", "global"} {
			for _, r := range rowsByArea[a] {
				r["area"] = a
			}
			selected = append(selected, rowsByArea[a]...)
		}
		body["sections"] = selected
		body["areas"] = map[string]any{
			"page": len(rowsByArea["page"]), "header": len(rowsByArea["header"]),
			"footer": len(rowsByArea["footer"]), "global": len(rowsByArea["global"]),
		}
	}

	if section != "" {
		row := findSectionRow(selected, section)
		if row == nil {
			return common.ExecResult{}, output.ErrValidation("section %q not found in area %q", section, area).
				WithHint("check the sections list of `themes +page` (or switch --area)")
		}
		if _, isPB := pbCustomID(getString(row, "type")); isPB {
			inc.Pb = true
		}
		selected = []map[string]any{row}
		body["sections"] = selected
	}

	if inc.Pb {
		if err := expandPbCanvas(ctx, in, selected); err != nil {
			return common.ExecResult{}, err
		}
	}
	if inc.Schema {
		types := map[string]bool{}
		for _, row := range selected {
			types[getString(row, "type")] = true
		}
		body["schema"] = projectSchemas(mapField(inner, "schemas"), types)
	}
	return common.ExecResult{Body: body}, nil
}

// pageDryRunPlans lists every intended request without sending any (strict
// zero-call + placeholders).
func pageDryRunPlans(themeID, session string, inc pageInclude) []common.PlannedRequest {
	themeRef := themeID
	var plans []common.PlannedRequest
	if themeRef == "" {
		themeRef = phThemeID
		plans = append(plans, PlanThemesList(map[string]any{"published": "1"}))
	}
	plans = append(plans, PlanDocTree(themeRef))
	oseidRef := session
	if oseidRef == "" {
		oseidRef = phOseid
		plans = append(plans, PlanCreateSession(themeRef)) // will create session
	}
	plans = append(plans, PlanSchemasList(oseidRef, phDocID))
	if inc.Pb {
		plans = append(plans, PlanPbBlocksGet(phCustomID, nil))
	}
	return plans
}

// pageList implements +page --list: standard templates from the doctree plus
// custom templates from list-templates, in one discovery payload.
func pageList(ctx context.Context, in common.ExecInput, themeID string) (common.ExecResult, error) {
	if in.DryRun {
		themeRef := themeID
		var plans []common.PlannedRequest
		if themeRef == "" {
			themeRef = phThemeID
			plans = append(plans, PlanThemesList(map[string]any{"published": "1"}))
		}
		plans = append(plans,
			PlanDocTree(themeRef),
			PlanListTemplates(themeRef, map[string]any{"per_page": "100"}),
		)
		return common.ExecResult{Plans: plans}, nil
	}

	if themeID == "" {
		resp, err := common.Send(ctx, in.Client, PlanThemesList(map[string]any{"published": "1"}))
		if err != nil {
			return common.ExecResult{}, err
		}
		if themeID = publishedThemeID(resp); themeID == "" {
			return common.ExecResult{}, output.ErrValidation("no published theme found").
				WithHint("pass --theme <theme_id> explicitly (see `themes list`)")
		}
	}

	// doctree and list-templates are independent reads — fetch concurrently,
	// keeping doctree's error precedence (it fails first, as it did serially).
	var customResp map[string]any
	var customErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		customResp, customErr = common.Send(ctx, in.Client, PlanListTemplates(themeID, map[string]any{"per_page": "100"}))
	}()
	treeResp, treeErr := common.Send(ctx, in.Client, PlanDocTree(themeID))
	<-done
	if treeErr != nil {
		return common.ExecResult{}, treeErr
	}
	if customErr != nil {
		return common.ExecResult{}, customErr
	}

	var templates []map[string]any
	for _, item := range doctreeGroupItems(treeResp, "templates") {
		location := getString(item, "location")
		name := strings.TrimSuffix(location, ".liquid")
		if name == "" || strings.Contains(name, ".") { // custom suffixed files come from list-templates
			continue
		}
		title := standardPageTitles[name]
		if title == "" {
			title = name
		}
		entry := map[string]any{"template": name, "type": "system", "title": title}
		if name == "index" {
			entry["url"] = "/"
		}
		templates = append(templates, entry)
	}

	root := customResp
	if d := mapField(customResp, "data"); d != nil {
		root = d
	}
	for _, item := range mapSlice(root["theme_templates"]) {
		typ := getString(item, "type")
		name := typ
		if suffix := getString(item, "suffix"); suffix != "" {
			name = typ + "." + suffix
		}
		entry := map[string]any{"template": name, "type": typ, "title": getString(item, "title")}
		if v := getString(item, "obj_title"); v != "" {
			entry["obj_title"] = v
		}
		if v := getString(item, "updated_at"); v != "" {
			entry["updated_at"] = v
		}
		templates = append(templates, entry)
	}
	return common.ExecResult{Body: map[string]any{"theme_id": themeID, "templates": templates}}, nil
}

// buildSectionRow converts one schemas-list card into the +page output row:
// stringified section_id, visible (from display, default true), current
// settings, and the flattened blocks with pre-built targets. PB custom cards
// are tagged kind:"pb" (canvas attaches later under --include pb).
func buildSectionRow(m map[string]any) map[string]any {
	id := anyToString(m["id"])
	typ := getString(m, "type")
	row := map[string]any{
		"section_id": id,
		"type":       typ,
		// set_visibility toggles "disabled"; "display" is a separate signal.
		"visible":  m["display"] != false && m["disabled"] != true,
		"settings": mapField(m, "settings"),
	}
	// name: pb cards carry the display name in schema.name (top-level name is
	// the slug); theme and app cards carry it in name. Passed through verbatim
	// (theme cards: string; pb/app cards: bilingual object); fixed cards have
	// none and the field is omitted.
	var name any
	if isPbType(typ) {
		if s := mapField(m, "schema"); s != nil {
			name = s["name"]
		}
	} else {
		name = m["name"]
	}
	if name != nil && name != "" {
		row["name"] = name
	}
	blocks, _ := m["blocks"].([]any)
	flat := flattenBlocks(id, blocks)
	rows := make([]map[string]any, 0, len(flat))
	for _, b := range flat {
		rows = append(rows, map[string]any{"type": b.Type, "settings": b.Settings, "target": b.Target})
	}
	row["blocks"] = rows
	if _, ok := pbCustomID(typ); ok {
		row["kind"] = "pb"
	}
	return row
}

func findSectionRow(rows []map[string]any, sectionID string) map[string]any {
	for _, row := range rows {
		if getString(row, "section_id") == sectionID {
			return row
		}
	}
	return nil
}

// expandPbCanvas fetches the canvas text for every kind:"pb" row in place
// (one pb-blocks-get per card, fetched concurrently — the GETs are independent;
// each goroutine writes only its own row). Per-card failures degrade to a
// canvas_error note instead of failing the read: theme-baked PB cards reference
// designer-side templates that 404 on the merchant store.
func expandPbCanvas(ctx context.Context, in common.ExecInput, rows []map[string]any) error {
	sem := make(chan struct{}, 4)
	var wg sync.WaitGroup
	for _, row := range rows {
		customID, ok := pbCustomID(getString(row, "type"))
		if !ok {
			continue
		}
		wg.Add(1)
		go func(row map[string]any, customID string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()
			resp, err := common.Send(ctx, in.Client, PlanPbBlocksGet(customID, nil))
			if err != nil {
				row["canvas_error"] = fmt.Sprintf("pb template %s unavailable on this store: %v", customID, err)
				return
			}
			root := resp
			if d := mapField(resp, "data"); d != nil {
				root = d
			}
			row["canvas"] = getString(root, "text")
		}(row, customID)
	}
	wg.Wait()
	return nil
}

// projectSchemas trims the bilingual card schemas down to a zh-CN projection
// for the card types present in the output.
func projectSchemas(schemas map[string]any, types map[string]bool) map[string]any {
	out := map[string]any{}
	for typ := range types {
		card := mapField(schemas, typ)
		if card == nil {
			continue
		}
		proj := map[string]any{}
		if settings, ok := card["settings"].([]any); ok {
			proj["settings"] = projectSettings(settings)
		}
		if maxBlocks, ok := card["max_blocks"]; ok {
			proj["max_blocks"] = maxBlocks
		}
		if blocks, ok := card["blocks"].([]any); ok {
			sub := map[string]any{}
			for _, b := range blocks {
				bm := asMap(b)
				if bm == nil {
					continue
				}
				btype := getString(bm, "type")
				if btype == "" {
					continue
				}
				entry := map[string]any{"label": zhText(bm["name"])}
				if bs, ok := bm["settings"].([]any); ok {
					entry["settings"] = projectSettings(bs)
				}
				sub[btype] = entry
			}
			proj["blocks"] = sub
		}
		out[typ] = proj
	}
	return out
}

// projectSettings keeps the semantic subset of a settings schema list:
// id/type/label/options/info/visibleOn/default/min/max/step/unit, labels
// collapsed to zh-CN (en-US fallback). Empty values are dropped.
func projectSettings(settings []any) []map[string]any {
	out := make([]map[string]any, 0, len(settings))
	for _, s := range settings {
		m := asMap(s)
		if m == nil || getString(m, "id") == "" {
			continue
		}
		p := map[string]any{"id": m["id"], "type": m["type"]}
		if l := zhText(m["label"]); l != "" {
			p["label"] = l
		}
		if info := zhText(m["info"]); info != "" {
			p["info"] = info
		}
		if opts, ok := m["options"].([]any); ok && len(opts) > 0 {
			po := make([]map[string]any, 0, len(opts))
			for _, o := range opts {
				om := asMap(o)
				if om == nil {
					continue
				}
				po = append(po, map[string]any{"value": om["value"], "label": zhText(om["label"])})
			}
			p["options"] = po
		}
		for _, k := range []string{"visibleOn", "default", "min", "max", "step", "unit"} {
			if v, ok := m[k]; ok && v != nil && v != "" {
				p[k] = v
			}
		}
		out = append(out, p)
	}
	return out
}

// zhText collapses a bilingual label ({zh-CN, en-US}) or plain string to one
// display string, preferring zh-CN.
func zhText(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case map[string]any:
		if s := getString(t, "zh-CN"); s != "" {
			return s
		}
		return getString(t, "en-US")
	default:
		return ""
	}
}

// extractOseid pulls the oseid out of a create-session response, tolerating
// an optional data wrapper.
func extractOseid(resp map[string]any) string {
	if s := getString(resp, "oseid"); s != "" {
		return s
	}
	if d := mapField(resp, "data"); d != nil {
		return getString(d, "oseid")
	}
	return ""
}

// anyToString renders a JSON scalar id (string or number) as a string;
// page_sections ids are integers, fixed card ids are strings.
func anyToString(v any) string {
	switch t := v.(type) {
	case string:
		return t
	case float64:
		if t == float64(int64(t)) {
			return fmt.Sprintf("%d", int64(t))
		}
		return fmt.Sprintf("%v", t)
	case nil:
		return ""
	default:
		return fmt.Sprintf("%v", t)
	}
}

// doctreeGroupItems returns one doctree group as maps, tolerating the same
// envelope shapes as docIDForLocation.
func doctreeGroupItems(resp map[string]any, group string) []map[string]any {
	tree := resp
	if d := mapField(resp, "data"); d != nil {
		if dt := mapField(d, "doctree"); dt != nil {
			tree = dt
		} else {
			tree = d
		}
	} else if dt := mapField(resp, "doctree"); dt != nil {
		tree = dt
	}
	return mapSlice(tree[group])
}
