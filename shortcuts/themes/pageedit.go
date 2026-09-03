package themes

import (
	"context"
	"io"
	"os"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/theme/doc"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// Shared foundation for themes +page / +edit: theme/doc resolution, schemas-list
// unwrapping and area grouping, PB card detection, session error classification.
// Both shortcuts consume this file; keep it flag-free.

// resolveThemeAndDoc resolves the working theme and template file id: empty
// themeID defaults to the published theme; template or file selects the doc.
func resolveThemeAndDoc(ctx context.Context, c *client.Client, themeID, template, file string) (string, string, error) {
	if themeID == "" {
		resp, err := common.Send(ctx, c, PlanThemesList(map[string]any{"published": "1"}))
		if err != nil {
			return "", "", err
		}
		themeID = publishedThemeID(resp)
		if themeID == "" {
			return "", "", output.ErrValidation("no published theme found").
				WithHint("pass --theme <theme_id> explicitly (see `themes list`)")
		}
	}
	group, location, err := templateLocation(template, file)
	if err != nil {
		return "", "", err
	}
	resp, err := common.Send(ctx, c, PlanDocTree(themeID))
	if err != nil {
		return "", "", err
	}
	if docID := docIDForLocation(resp, group, location); docID != "" {
		return themeID, docID, nil
	}
	// A published theme's doctree is its live snapshot, so a custom template
	// created through the API — which lives only on the draft until the theme
	// is published — is missing from it. list-templates carries every custom
	// template with both its suffix and its doc_id, which is exactly what the
	// name resolves to. (`themes +page --list` already merges both sources;
	// resolving from doctree alone is what made a listed template unusable.)
	if group == "templates" {
		custom, cerr := common.Send(ctx, c, PlanListTemplates(themeID, map[string]any{"per_page": "100"}))
		if cerr != nil {
			return "", "", cerr
		}
		if docID := docIDForCustomTemplate(custom, location); docID != "" {
			return themeID, docID, nil
		}
	}
	return "", "", output.ErrValidation("template file %q not found in theme %s", location, themeID).
		WithHint("run `themes +page --list` to discover available templates")
}

// docIDForCustomTemplate maps a "<type>.<suffix>.liquid" location onto a custom
// template's doc_id from a list-templates response.
func docIDForCustomTemplate(resp map[string]any, location string) string {
	root := unwrapData(resp)
	want := strings.TrimSuffix(location, ".liquid")
	for _, item := range mapSlice(root["theme_templates"]) {
		name := getString(item, "type")
		if suffix := getString(item, "suffix"); suffix != "" {
			name += "." + suffix
		}
		if name == want {
			return getString(item, "doc_id")
		}
	}
	return ""
}

// templateLocation maps the --template / --file flag pair (exactly one set)
// to a doctree (group, location) pair.
func templateLocation(template, file string) (string, string, error) {
	switch {
	case template != "" && file != "":
		return "", "", output.ErrValidation("--template and --file are mutually exclusive")
	case template != "":
		return "templates", template + ".liquid", nil
	case file != "":
		typ, location, err := doc.ParseThemeFile(file)
		if err != nil {
			return "", "", output.ErrValidation("invalid --file path: %v", err)
		}
		return doctreeGroup(typ), location, nil
	default:
		return "", "", output.ErrValidation("one of --template or --file is required")
	}
}

// doctreeGroup maps a canonical theme file type to its doctree response key
// (the response pluralizes config/layout; other types match their dir name).
func doctreeGroup(typ string) string {
	switch typ {
	case "config":
		return "configs"
	case "layout":
		return "layouts"
	default:
		return typ
	}
}

// publishedThemeID extracts the first theme id from a GET /themes response,
// tolerating an optional data wrapper.
func publishedThemeID(resp map[string]any) string {
	root := unwrapData(resp)
	items, _ := root["themes"].([]any)
	for _, it := range items {
		if m := asMap(it); m != nil {
			if id := getString(m, "id"); id != "" {
				return id
			}
		}
	}
	return ""
}

// docIDForLocation finds the file id for (group, location) in a doctree
// response, tolerating {data:{doctree:{...}}}, {data:{...}} and bare shapes.
func docIDForLocation(resp map[string]any, group, location string) string {
	for _, m := range doctreeGroupItems(resp, group) {
		if getString(m, "location") == location {
			return getString(m, "id")
		}
	}
	return ""
}

// fetchSections sends schemas-list and returns its inner {schemas, sections}
// payload, unwrapping up to two data envelopes.
func fetchSections(ctx context.Context, c *client.Client, oseid, docID string) (map[string]any, error) {
	resp, err := common.Send(ctx, c, PlanSchemasList(oseid, docID))
	if err != nil {
		return nil, err
	}
	inner := resp
	for i := 0; i < 2; i++ { // tolerate up to two data wrappers
		if _, ok := inner["sections"]; ok {
			break
		}
		if d := mapField(inner, "data"); d != nil {
			inner = d
		} else {
			break
		}
	}
	if _, ok := inner["sections"]; !ok {
		return nil, output.ErrInternal("unexpected schemas-list response: no sections payload")
	}
	return inner, nil
}

// splitSections splits the schemas-list sections payload into page-flow cards
// (page_sections) and fixed cards (sections: header/footer/announcement, by id).
func splitSections(inner map[string]any) (page []map[string]any, fixed []map[string]any) {
	sec := mapField(inner, "sections")
	if sec == nil {
		return nil, nil
	}
	return mapSlice(sec["page_sections"]), mapSlice(sec["sections"])
}

// areaOf maps a fixed card's id to its +page --area bucket; anything but
// header/footer falls into global.
func areaOf(fixedID string) string {
	switch fixedID {
	case "header", "footer":
		return fixedID
	default:
		return "global"
	}
}

// sectionsByArea groups a schemas-list payload into page/header/footer/global;
// the fixed-cards key varies ("sections" or "global_sections") — both fold in.
func sectionsByArea(inner map[string]any) map[string][]map[string]any {
	out := map[string][]map[string]any{"page": {}, "header": {}, "footer": {}, "global": {}}
	sec := mapField(inner, "sections")
	if sec == nil {
		return out
	}
	out["page"] = mapSlice(sec["page_sections"])
	out["header"] = append(out["header"], mapSlice(sec["header_sections"])...)
	out["footer"] = append(out["footer"], mapSlice(sec["footer_sections"])...)
	for _, key := range []string{"sections", "global_sections"} {
		for _, m := range mapSlice(sec[key]) { // fixed cards: classify by id
			a := areaOf(anyToString(m["id"]))
			out[a] = append(out[a], m)
		}
	}
	return out
}

// allSections flattens every area's sections into one slice.
func allSections(inner map[string]any) []map[string]any {
	ba := sectionsByArea(inner)
	var all []map[string]any
	for _, a := range []string{"page", "header", "footer", "global"} {
		all = append(all, ba[a]...)
	}
	return all
}

// mapSlice converts a []any of objects to []map[string]any, skipping non-maps.
func mapSlice(v any) []map[string]any {
	items, _ := v.([]any)
	out := make([]map[string]any, 0, len(items))
	for _, it := range items {
		if m := asMap(it); m != nil {
			out = append(out, m)
		}
	}
	return out
}

// pbTemplateTypeRe matches either PB card family and captures its scope plus
// template id ({family}-{N} in .../page-builder/blocks/{family}-{N}).
var pbTemplateTypeRe = regexp.MustCompile(`page-builder/blocks/(custom|global)-(\d+)`)

// pbTemplateRef extracts a PB card's bare template id and the scope the summary
// endpoint calls "type" (custom | global). Returns ok=false for theme cards,
// app blocks, and PB cards whose type carries no numeric template id.
func pbTemplateRef(sectionType string) (id, scope string, ok bool) {
	m := pbTemplateTypeRe.FindStringSubmatch(sectionType)
	if m == nil {
		return "", "", false
	}
	return m[2], m[1], true
}

// isPbType reports whether a section type belongs to the page-builder app
// (custom- and global- families), including types carrying no template id.
func isPbType(sectionType string) bool {
	return strings.HasPrefix(sectionType, "shoplazza://apps/page-builder/")
}

// isSessionNotFound reports whether an API error means the edit session is gone
// (never auto-recreate); an invalid oseid surfaces as a 500 with "b_invalid_themeid".
func isSessionNotFound(err error) bool {
	if err == nil {
		return false
	}
	msg := err.Error()
	return strings.Contains(msg, "SESSION_NOT_FOUND") || strings.Contains(msg, "b_invalid_themeid")
}

// readFlagInput resolves a flag value naming its content: "-" is stdin, a value
// starting with an inlinePrefixes byte is literal, anything else is a file path.
func readFlagInput(flag, val string, inlinePrefixes ...byte) ([]byte, error) {
	switch {
	case val == "":
		return nil, nil
	case val == "-":
		raw, err := io.ReadAll(os.Stdin)
		if err != nil {
			return nil, output.ErrValidation("reading %s from stdin: %v", flag, err)
		}
		return raw, nil
	}
	if trimmed := strings.TrimSpace(val); trimmed != "" {
		for _, p := range inlinePrefixes {
			if trimmed[0] == p {
				return []byte(val), nil
			}
		}
	}
	raw, err := os.ReadFile(val)
	if err != nil {
		return nil, output.ErrValidation("reading %s file: %v", flag, err)
	}
	return raw, nil
}

// previewURLLater starts the two preview-URL lookups concurrently and returns
// the assembler. Buffered, so an early return never blocks the goroutines.
func previewURLLater(ctx context.Context, c *client.Client, template, file string) func(themeID, oseid string) string {
	domainCh := make(chan string, 1)
	go func() { domainCh <- extractStoreDomainBest(ctx, c) }()
	pathCh := make(chan string, 1)
	go func() { pathCh <- resolvePreviewPath(ctx, c, template, file) }()
	return func(themeID, oseid string) string {
		return buildPreviewURL(<-domainCh, <-pathCh, themeID, oseid, "")
	}
}

// sectionIDSet indexes the page's current section ids, to diff a later read
// against (the server assigns ids and ignores client-supplied ones).
func sectionIDSet(inner map[string]any) map[string]bool {
	ids := map[string]bool{}
	for _, m := range allSections(inner) {
		if id := anyToString(m["id"]); id != "" {
			ids[id] = true
		}
	}
	return ids
}

// newSectionIDs returns the ids present now but absent from before, in page
// order — the sections the batch just created.
func newSectionIDs(inner map[string]any, before map[string]bool) []string {
	var out []string
	for _, m := range allSections(inner) {
		if id := anyToString(m["id"]); id != "" && !before[id] {
			out = append(out, id)
		}
	}
	return out
}

// dryRunThemeRef resolves the theme for a dry-run plan list: an explicit id is
// used as is, an empty one becomes the placeholder plus the lookup resolving it.
func dryRunThemeRef(themeID string) (string, []common.PlannedRequest) {
	if themeID != "" {
		return themeID, nil
	}
	return phThemeID, []common.PlannedRequest{PlanThemesList(map[string]any{"published": "1"})}
}

// unwrapData strips one {data:{…}} envelope; responses arrive both ways.
func unwrapData(resp map[string]any) map[string]any {
	if d := mapField(resp, "data"); d != nil {
		return d
	}
	return resp
}

// doctreeRoot locates the file tree in a doctree response, tolerating
// {data:{doctree:{…}}}, {data:{…}}, {doctree:{…}} and bare shapes.
func doctreeRoot(resp map[string]any) map[string]any {
	if d := mapField(resp, "data"); d != nil {
		if dt := mapField(d, "doctree"); dt != nil {
			return dt
		}
		return d
	}
	if dt := mapField(resp, "doctree"); dt != nil {
		return dt
	}
	return resp
}

// themeInfoForDryRun reads the local theme name/version for a dry-run plan,
// substituting placeholders when the directory carries neither.
func themeInfoForDryRun(cwd string) (string, string) {
	name, version, _ := readThemeInfo(cwd)
	if name == "" {
		name = "<theme>"
	}
	if version == "" {
		version = "<version>"
	}
	return name, version
}
