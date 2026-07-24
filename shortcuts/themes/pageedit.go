package themes

import (
	"context"
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
	docID := docIDForLocation(resp, group, location)
	if docID == "" {
		return "", "", output.ErrValidation("template file %q not found in theme %s", location, themeID).
			WithHint("run `themes +page --list` to discover available templates")
	}
	return themeID, docID, nil
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
	root := resp
	if d := mapField(resp, "data"); d != nil {
		root = d
	}
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
	items, _ := tree[group].([]any)
	for _, it := range items {
		m := asMap(it)
		if m == nil {
			continue
		}
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

// pbCustomTypeRe matches a page-builder custom card's section type and
// captures its custom template id ({N} in .../page-builder/blocks/custom-{N}).
var pbCustomTypeRe = regexp.MustCompile(`page-builder/blocks/custom-(\d+)`)

// pbCustomID extracts the custom template id from a PB card's section type.
// Returns ("", false) for theme cards and public app blocks.
func pbCustomID(sectionType string) (string, bool) {
	m := pbCustomTypeRe.FindStringSubmatch(sectionType)
	if m == nil {
		return "", false
	}
	return m[1], true
}

// isPbType reports whether a section type belongs to the page-builder app
// (custom- and global- families) — broader than pbCustomID's custom-only capture.
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
