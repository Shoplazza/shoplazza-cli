package themes

// themes +card-schema — batch schema lookup for addable theme cards.
//
// For each theme-source card id it pulls sections/<id>.liquid via the doc
// endpoint and extracts the {% schema %} JSON locally. Default output is the
// compact zh-CN projection shared with "+page --include schema" (context
// cost: roughly half the verbatim schema); --full keeps the bilingual
// original. Requests run strictly serially — the sections endpoint
// family 500s under concurrency (filed 2026-07-30) and the batch semantics
// exist precisely to avoid that.
//
// Extension ids (shoplazza:// URIs) have no theme file: they return
// {id, source, settings: []} without a request. Unresolvable ids collect in
// "missing" instead of failing the batch.
//
// Phase 1 stopgap: retire once the backend batch schema endpoint ships.

import (
	"context"
	"encoding/json"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

const cardSchemaMaxIDs = 10

var (
	schemaOpenRe  = regexp.MustCompile(`\{%-?\s*schema\s*-?%\}`)
	schemaCloseRe = regexp.MustCompile(`\{%-?\s*endschema\s*-?%\}`)
)

// extractSchema pulls the {% schema %} JSON out of a section liquid source.
func extractSchema(content string) (map[string]any, error) {
	open := schemaOpenRe.FindStringIndex(content)
	if open == nil {
		return nil, output.ErrValidation("no {%% schema %%} tag in section source")
	}
	rest := content[open[1]:]
	close := schemaCloseRe.FindStringIndex(rest)
	if close == nil {
		return nil, output.ErrValidation("missing {%% endschema %%} tag")
	}
	var schema map[string]any
	if err := json.Unmarshal([]byte(rest[:close[0]]), &schema); err != nil {
		return nil, err
	}
	return schema, nil
}

var cardSchemaShortcut = common.Shortcut{
	Service: "themes",
	Command: "+card-schema",
	Use:     "+card-schema",
	Short:   "Batch-read the settings/blocks schema of addable theme cards by id",
	Long: `Read the configuration schema (settings and blocks) of up to 10 addable
theme cards in one call. Card ids come from "themes section cards" /
"themes +cards".

By default the output is the same compact zh-CN projection "+page --include
schema" uses: per setting id/type/label/info/options/default/min/max/step/
unit, blocks keyed by type. Pass --full for the verbatim bilingual schema
(keeps en-US labels and paragraph entries).

Theme-source ids resolve to sections/<id>.liquid and the {% schema %} JSON is
extracted locally; a case-insensitive doctree fallback covers themes whose
card type and file name differ in casing. Extension ids (shoplazza:// URIs)
have no theme file and return {id, source, settings: []} — keep their name
from the cards listing. Ids that resolve nowhere are listed in "missing"
without failing the rest of the batch.

Presets are omitted by default (bulky, not needed for recommendation);
pass --include-presets to keep them.`,
	Flags: []common.Flag{
		{Name: "theme", Type: common.FlagString, Required: true, Description: "Theme ID (same value as themes section cards)."},
		{Name: "ids", Type: common.FlagStringSlice, Required: true, Description: "Card ids, comma-separated, at most 10."},
		{Name: "full", Type: common.FlagBool, Description: "Return the verbatim bilingual schema instead of the compact zh-CN projection."},
		{Name: "include-presets", Type: common.FlagBool, Description: "Include the schema presets block (omitted by default)."},
	},
	Execute: cardSchemaExecute,
}

func cardSchemaExecute(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
	themeID := in.Flags.GetString("theme")
	if themeID == "" {
		return common.ExecResult{}, output.ErrValidation("--theme is required")
	}
	ids := in.Flags.GetStringSlice("ids")
	if len(ids) == 0 {
		return common.ExecResult{}, output.ErrValidation("--ids is required")
	}
	if len(ids) > cardSchemaMaxIDs {
		return common.ExecResult{}, output.ErrValidation("at most %d ids per call, got %d", cardSchemaMaxIDs, len(ids)).
			WithHint("split the candidates into batches of 10")
	}
	includePresets := in.Flags.GetBool("include-presets")
	full := in.Flags.GetBool("full")

	if in.DryRun {
		var plans []common.PlannedRequest
		for _, id := range ids {
			if strings.HasPrefix(id, "shoplazza://") {
				continue // extension cards have no theme file; answered locally
			}
			plans = append(plans, PlanDocGet(themeID, docQuery(id+".liquid")))
		}
		return common.ExecResult{Plans: plans}, nil
	}

	items := make([]map[string]any, 0, len(ids))
	missing := make([]string, 0)
	var sectionsByLower map[string]string // lazy doctree fallback, one fetch per run

	for _, id := range ids {
		if strings.HasPrefix(id, "shoplazza://") {
			items = append(items, map[string]any{"id": id, "source": "extension", "settings": []any{}})
			continue
		}
		content, err := fetchSectionSource(ctx, in, themeID, id+".liquid")
		if err != nil {
			// Resolve casing drift (e.g. card type "Timeline" vs timeline.liquid)
			// through the doctree, fetched at most once per command run.
			if sectionsByLower == nil {
				sectionsByLower, err = sectionLocations(ctx, in, themeID)
				if err != nil {
					return common.ExecResult{}, err
				}
			}
			actual := sectionsByLower[strings.ToLower(id)+".liquid"]
			if actual == "" || actual == id+".liquid" {
				missing = append(missing, id)
				continue
			}
			if content, err = fetchSectionSource(ctx, in, themeID, actual); err != nil {
				missing = append(missing, id)
				continue
			}
		}
		schema, err := extractSchema(content)
		if err != nil {
			missing = append(missing, id)
			continue
		}
		var item map[string]any
		if full {
			item = map[string]any{
				"id":         id,
				"source":     "theme",
				"name":       schema["name"],
				"max_blocks": schema["max_blocks"],
				"settings":   orEmptyList(schema["settings"]),
				"blocks":     orEmptyList(schema["blocks"]),
			}
		} else {
			item = projectCardSchema(id, schema)
		}
		if includePresets {
			item["presets"] = schema["presets"]
		}
		items = append(items, item)
	}

	return common.ExecResult{Body: map[string]any{"items": items, "missing": missing}}, nil
}

// projectCardSchema compresses one parsed {% schema %} into the same compact
// zh-CN shape "+page --include schema" emits (projectSettings), so agents deal
// with a single schema format across both commands. Unlike the schemas-list
// payload, liquid schema block entries carry their fields inline, so blocks
// project directly without the sibling-schema lookup projectBlock does.
func projectCardSchema(id string, schema map[string]any) map[string]any {
	item := map[string]any{"id": id, "source": "theme"}
	if n := zhText(schema["name"]); n != "" {
		item["name"] = n
	} else if schema["name"] != nil {
		item["name"] = schema["name"]
	}
	if v := schema["max_blocks"]; v != nil {
		item["max_blocks"] = v
	}
	settings, _ := schema["settings"].([]any)
	item["settings"] = projectSettings(settings)

	blocks := map[string]any{}
	list, _ := schema["blocks"].([]any)
	for _, b := range list {
		bm := asMap(b)
		if bm == nil {
			continue
		}
		btype := getString(bm, "type")
		if btype == "" {
			continue
		}
		entry := map[string]any{}
		if btype == appBlockSentinel {
			entry["app_blocks"] = true
		}
		if l := zhText(bm["name"]); l != "" {
			entry["label"] = l
		}
		if bs, ok := bm["settings"].([]any); ok {
			entry["settings"] = projectSettings(bs)
		}
		blocks[btype] = entry
	}
	item["blocks"] = blocks
	return item
}

func docQuery(location string) map[string]any {
	return map[string]any{"type": "sections", "location": location}
}

// fetchSectionSource reads one sections/<location> file and returns its liquid source.
func fetchSectionSource(ctx context.Context, in common.ExecInput, themeID, location string) (string, error) {
	resp, err := common.Send(ctx, in.Client, PlanDocGet(themeID, docQuery(location)))
	if err != nil {
		return "", err
	}
	root := resp
	if d := mapField(resp, "data"); d != nil {
		root = d
	}
	content := getString(mapField(root, "theme_file"), "content")
	if content == "" {
		return "", output.ErrValidation("empty content for %s", location)
	}
	return content, nil
}

// sectionLocations fetches the doctree once and indexes sections file names by
// their lowercased form, for case-insensitive id → file resolution.
func sectionLocations(ctx context.Context, in common.ExecInput, themeID string) (map[string]string, error) {
	resp, err := common.Send(ctx, in.Client, PlanDocTree(themeID))
	if err != nil {
		return nil, err
	}
	root := resp
	if d := mapField(resp, "data"); d != nil {
		root = d
	}
	out := map[string]string{}
	entries, _ := root["sections"].([]any)
	for _, e := range entries {
		if loc := getString(asMap(e), "location"); loc != "" {
			out[strings.ToLower(loc)] = loc
		}
	}
	return out, nil
}

func orEmptyList(v any) any {
	if v == nil {
		return []any{}
	}
	return v
}
