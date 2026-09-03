package themes

import (
	"encoding/json"
	"errors"
	"fmt"
	"regexp"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// Shared helpers for themes block +edit / +get (AI-generated block files).

const (
	genTypePrefix   = "blocks/"
	genIDPrefix     = "gen_"
	genSectionType  = "_blocks"
	phGenBlockType  = "<gen_block_type>"
	phGenSettings   = "<gen_block_settings>"
	phCurrentValues = "<current_instance_settings>"
)

// normalizeGenType accepts "gen_x" or "blocks/gen_x" and returns the card
// type ("blocks/gen_x") plus the bare id ("gen_x").
func normalizeGenType(id string) (string, string, error) {
	bare := strings.TrimPrefix(strings.TrimSpace(id), genTypePrefix)
	if bare == "" || !strings.HasPrefix(bare, genIDPrefix) || strings.ContainsAny(bare, "/ .") {
		return "", "", output.ErrValidation("invalid block id %q", id).
			WithHint("the id is the block file name without extension, e.g. gen_1a0d523 (blocks/gen_1a0d523 is accepted too)")
	}
	return genTypePrefix + bare, bare, nil
}

var dotIndexRe = regexp.MustCompile(`\.blocks\.(\d+)`)

// normalizeBlockTarget accepts the server's dot-index grammar (sid.blocks.0)
// and renders it in the +page grammar (sid.blocks[0]); bracket input is unchanged.
func normalizeBlockTarget(target string) string {
	return dotIndexRe.ReplaceAllString(strings.TrimSpace(target), ".blocks[$1]")
}

// readContentInput loads the liquid source. Inline source is not accepted:
// quotes and newlines do not survive a shell round-trip reliably.
func readContentInput(val string) (string, error) {
	if val == "" {
		return "", output.ErrValidation("--content is required").
			WithHint("pass the liquid source as a file path, or '-' to read it from stdin")
	}
	raw, err := readFlagInput("--content", val)
	return string(raw), err
}

// readJSONObjectInput loads a JSON object flag. Empty input yields nil.
func readJSONObjectInput(flag, val string) (map[string]any, error) {
	raw, err := readFlagInput(flag, val, '{', '[')
	if err != nil || raw == nil {
		return nil, err
	}
	var out map[string]any
	if err := json.Unmarshal(raw, &out); err != nil {
		return nil, output.ErrValidation("%s must be a JSON object: %v", flag, err)
	}
	return out, nil
}

// countStdin reports how many flag values ask to read stdin.
func countStdin(vals ...string) int {
	n := 0
	for _, v := range vals {
		if v == "-" {
			n++
		}
	}
	return n
}

// unwrapData strips the {data:{…}} envelope from a gen-blocks response.
func unwrapData(resp map[string]any) map[string]any {
	if d := mapField(resp, "data"); d != nil {
		return d
	}
	return resp
}

// genInstance is one placement of a gen block: the template holding it and
// the block target in +page grammar.
type genInstance struct {
	Template string
	Target   string
}

// parseGenInstances flattens the instances payload into one row per placement,
// accepting both shapes seen: a list of {template: [paths]}, or one such object.
func parseGenInstances(v any) []genInstance {
	groups := mapSlice(v)
	if m := asMap(v); m != nil {
		groups = []map[string]any{m}
	}
	var out []genInstance
	for _, group := range groups {
		for tmpl, paths := range group {
			items, _ := paths.([]any)
			for _, p := range items {
				if s, ok := p.(string); ok && s != "" {
					out = append(out, genInstance{Template: tmpl, Target: normalizeBlockTarget(s)})
				}
			}
		}
	}
	return out
}

// genInstanceRows renders instances for output.
func genInstanceRows(list []genInstance) []map[string]any {
	rows := make([]map[string]any, 0, len(list))
	for _, it := range list {
		rows = append(rows, map[string]any{"template": it.Template, "target": it.Target})
	}
	return rows
}

// schemaSettingIDs collects the setting ids declared by a block schema.
func schemaSettingIDs(schema map[string]any) map[string]bool {
	ids := map[string]bool{}
	for _, s := range mapSlice(schema["settings"]) {
		if id := getString(s, "id"); id != "" {
			ids[id] = true
		}
	}
	return ids
}

// presetSettings returns the schema's default instance values: presets[0].settings,
// falling back to each setting's default.
func presetSettings(schema map[string]any) map[string]any {
	out := map[string]any{}
	if presets := mapSlice(schema["presets"]); len(presets) > 0 {
		for k, v := range mapField(presets[0], "settings") {
			out[k] = v
		}
		if len(out) > 0 {
			return out
		}
	}
	for _, s := range mapSlice(schema["settings"]) {
		if id := getString(s, "id"); id != "" {
			if d, ok := s["default"]; ok {
				out[id] = d
			}
		}
	}
	return out
}

// migrateSettings carries the instance's current values onto the new schema:
// new-schema defaults, overlaid with current values whose key still exists.
func migrateSettings(defaults map[string]any, ids map[string]bool, current map[string]any) map[string]any {
	out := map[string]any{}
	for k, v := range defaults {
		out[k] = v
	}
	for k, v := range current {
		if len(ids) == 0 || ids[k] {
			out[k] = v
		}
	}
	return out
}

// schemaDisplayName picks the merchant-visible name: presets[0].cname, else the schema name.
func schemaDisplayName(schema map[string]any) any {
	if presets := mapSlice(schema["presets"]); len(presets) > 0 {
		if c, ok := presets[0]["cname"]; ok && c != nil {
			return c
		}
	}
	if n := getString(schema, "name"); n != "" {
		return n
	}
	return nil
}

// blockStageErr tags a request failure with the stage it happened in.
func blockStageErr(err error, stage, oseid string) error {
	var httpErr *client.HTTPError
	if errors.As(err, &httpErr) {
		return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "").
			WithEndpoint(httpErr.Method, httpErr.Path).
			WithField("stage", stage).WithField("oseid", oseid)
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) {
		return exitErr.WithField("stage", stage).WithField("oseid", oseid)
	}
	return err
}

// blockPlaceFailErr reports a written block file whose page placement failed:
// the file persists, so the caller re-runs placement only (or reverts).
func blockPlaceFailErr(oseid, cardType, revertID string, results []map[string]any, failed []int) *output.ExitError {
	return output.Errorf(output.ExitAPI, output.TypeAPI, "block file written but %d of %d placement ops failed", len(failed), len(results)).
		WithField("stage", "place").
		WithField("block_type", cardType).
		WithField("revert_id", revertID).
		WithField("oseid", oseid).
		WithField("results", results).
		WithField("failed", failed).
		WithHint(fmt.Sprintf("the block file is saved in the session; re-run placement with --id %s --template <name> --target <path>, or undo the write: themes block revert-gen --params '{\"oseid\":\"%s\"}' --data '{\"revert_id\":\"%s\"}'",
			strings.TrimPrefix(cardType, genTypePrefix), oseid, revertID))
}
