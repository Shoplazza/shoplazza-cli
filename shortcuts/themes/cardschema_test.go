package themes

import (
	"context"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"

	"github.com/spf13/cobra"
)

const sectionFixture = `<div>{{ section.settings.heading }}</div>
{% style %}.x{color:red}{% endstyle %}
{% schema %}
{
  "name": "hero_slideshow",
  "max_blocks": 5,
  "settings": [
    {"type": "checkbox", "id": "full_page", "label": {"en-US": "Full page width", "zh-CN": "全屏宽度"}, "default": true}
  ],
  "blocks": [
    {"type": "slide", "name": "slide", "settings": [{"type": "range", "id": "mask", "min": 0, "max": 100, "step": 2}]}
  ],
  "presets": [{"category": {"zh-CN": "幻灯"}}]
}
{% endschema %}`

func TestExtractSchema(t *testing.T) {
	s, err := extractSchema(sectionFixture)
	if err != nil {
		t.Fatalf("extract: %v", err)
	}
	if s["name"] != "hero_slideshow" || s["max_blocks"] != float64(5) {
		t.Errorf("unexpected schema head: name=%v max_blocks=%v", s["name"], s["max_blocks"])
	}
	if len(s["settings"].([]any)) != 1 || len(s["blocks"].([]any)) != 1 {
		t.Errorf("settings/blocks not passed through: %v", s)
	}

	// whitespace-control variant
	if _, err := extractSchema("{%- schema -%}{\"name\":\"x\"}{%- endschema -%}"); err != nil {
		t.Errorf("whitespace-control tags must parse: %v", err)
	}
	// failure modes
	if _, err := extractSchema("<div>no schema here</div>"); err == nil {
		t.Error("want error when tag missing")
	}
	if _, err := extractSchema("{% schema %}{\"name\": broken"); err == nil {
		t.Error("want error when endschema missing")
	}
	if _, err := extractSchema("{% schema %}{broken json}{% endschema %}"); err == nil {
		t.Error("want error on invalid JSON")
	}
}

func cardSchemaFlags(theme string, ids []string, presets bool) common.FlagSet {
	cmd := &cobra.Command{Use: "+card-schema"}
	cmd.Flags().String("theme", theme, "")
	cmd.Flags().StringSlice("ids", ids, "")
	cmd.Flags().Bool("include-presets", presets, "")
	return common.NewCobraFlagSet(cmd)
}

func TestCardSchemaValidation(t *testing.T) {
	ctx := context.Background()
	if _, err := cardSchemaShortcut.Execute(ctx, common.ExecInput{DryRun: true, Flags: cardSchemaFlags("", []string{"a"}, false)}); err == nil {
		t.Error("want error without --theme")
	}
	if _, err := cardSchemaShortcut.Execute(ctx, common.ExecInput{DryRun: true, Flags: cardSchemaFlags("abc", nil, false)}); err == nil {
		t.Error("want error without --ids")
	}
	eleven := make([]string, 11)
	for i := range eleven {
		eleven[i] = "c" + strings.Repeat("x", i)
	}
	if _, err := cardSchemaShortcut.Execute(ctx, common.ExecInput{DryRun: true, Flags: cardSchemaFlags("abc", eleven, false)}); err == nil {
		t.Error("want error above 10 ids")
	}
}

// TestSnapshot_CardSchemaDryRun locks the per-id doc GET plan shape; the
// extension URI contributes no plan (answered locally without a request).
func TestSnapshot_CardSchemaDryRun(t *testing.T) {
	ids := []string{"hero_slideshow", "icon_text", "shoplazza://apps/publicapp/blocks/popup/123"}
	res, err := cardSchemaShortcut.Execute(context.Background(), common.ExecInput{DryRun: true, Flags: cardSchemaFlags("abc", ids, false)})
	if err != nil {
		t.Fatalf("Execute err: %v", err)
	}
	if len(res.Plans) != 2 {
		t.Fatalf("extension id must not plan a request: %d plans", len(res.Plans))
	}
	snapshot(t, "card_schema_dry_run", plansToMap(res.Plans))
}

func TestProjectCardSchema(t *testing.T) {
	schema := map[string]any{
		"name":       map[string]any{"en-US": "Hero slideshow", "zh-CN": "图文幻灯"},
		"max_blocks": float64(5),
		"settings": []any{
			map[string]any{"type": "checkbox", "id": "full_page",
				"label": map[string]any{"en-US": "Full page width", "zh-CN": "全屏宽度"}, "default": true},
			map[string]any{"type": "paragraph",
				"content": map[string]any{"zh-CN": "说明文字"}}, // no id → dropped by projection
		},
		"blocks": []any{
			map[string]any{"type": "slide", "name": map[string]any{"zh-CN": "幻灯"},
				"settings": []any{map[string]any{"type": "range", "id": "mask",
					"label": map[string]any{"en-US": "Text protection", "zh-CN": "图片蒙层"},
					"min":   float64(0), "max": float64(100), "step": float64(2), "unit": "%"}}},
			map[string]any{"type": "@app"},
		},
	}

	got := projectCardSchema("hero_slideshow", schema)
	if got["name"] != "图文幻灯" || got["max_blocks"] != float64(5) {
		t.Errorf("head fields: name=%v max_blocks=%v", got["name"], got["max_blocks"])
	}
	settings := got["settings"].([]map[string]any)
	if len(settings) != 1 || settings[0]["label"] != "全屏宽度" || settings[0]["default"] != true {
		t.Errorf("settings projection wrong: %v", settings)
	}
	blocks := got["blocks"].(map[string]any)
	slide := blocks["slide"].(map[string]any)
	if slide["label"] != "幻灯" {
		t.Errorf("block label: %v", slide["label"])
	}
	ms := slide["settings"].([]map[string]any)[0]
	if ms["label"] != "图片蒙层" || ms["min"] != float64(0) || ms["unit"] != "%" {
		t.Errorf("block setting projection wrong: %v", ms)
	}
	if _, hasEn := ms["en-US"]; hasEn {
		t.Error("en-US must not survive the projection")
	}
	app := blocks["@app"].(map[string]any)
	if app["app_blocks"] != true {
		t.Errorf("@app sentinel: %v", app)
	}
}
