package themes

import (
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"
)

// nested fixture: root blocks[0] is a group with two children, the second of
// which has one grandchild; blocks[1] is a plain sibling.
func nestedBlocksFixture() []any {
	return []any{
		map[string]any{"type": "group", "settings": map[string]any{"gap": 8.0}, "blocks": []any{
			map[string]any{"type": "image", "settings": map[string]any{"src": "a.png"}},
			map[string]any{"type": "text", "settings": map[string]any{"content": "hi"}, "blocks": []any{
				map[string]any{"type": "icon", "settings": map[string]any{}},
			}},
		}},
		map[string]any{"type": "slide", "settings": map[string]any{"title": "s"}},
	}
}

func TestFlattenBlocks_DepthFirstTargets(t *testing.T) {
	rows := flattenBlocks("sec_1", nestedBlocksFixture())

	wantTargets := []string{
		"sec_1.blocks[0]",
		"sec_1.blocks[0].blocks[0]",
		"sec_1.blocks[0].blocks[1]",
		"sec_1.blocks[0].blocks[1].blocks[0]",
		"sec_1.blocks[1]",
	}
	wantTypes := []string{"group", "image", "text", "icon", "slide"}

	if len(rows) != len(wantTargets) {
		t.Fatalf("rows = %d, want %d", len(rows), len(wantTargets))
	}
	for i, r := range rows {
		if r.Target != wantTargets[i] {
			t.Errorf("row %d target = %q, want %q", i, r.Target, wantTargets[i])
		}
		if r.Type != wantTypes[i] {
			t.Errorf("row %d type = %q, want %q", i, r.Type, wantTypes[i])
		}
	}
}

// Every target flattenBlocks produces must parse back to coordinates that
// locate the same block in the original tree (round-trip identity).
func TestFlattenBlocks_ParseTargetRoundTrip(t *testing.T) {
	blocks := nestedBlocksFixture()
	for _, row := range flattenBlocks("sec_1", blocks) {
		ref, err := parseTarget(row.Target)
		if err != nil {
			t.Fatalf("parseTarget(%q): %v", row.Target, err)
		}
		if ref.Kind != targetBlock || ref.SectionID != "sec_1" {
			t.Fatalf("parseTarget(%q) = %+v, want block ref in sec_1", row.Target, ref)
		}
		// Walk the original tree by the parsed coordinates.
		items := blocks
		for _, p := range ref.ParentPath {
			m := items[p].(map[string]any)
			items = m["blocks"].([]any)
		}
		got := items[ref.BlockIndex].(map[string]any)["type"].(string)
		if got != row.Type {
			t.Errorf("target %q resolves to type %q, want %q", row.Target, got, row.Type)
		}
	}
}

func TestParseTarget(t *testing.T) {
	cases := []struct {
		in      string
		want    targetRef
		wantErr bool
	}{
		{in: "sec_1", want: targetRef{SectionID: "sec_1", BlockIndex: -1, Kind: targetSection}},
		{in: "1638950411341", want: targetRef{SectionID: "1638950411341", BlockIndex: -1, Kind: targetSection}},
		{in: "sec_1.blocks[0]", want: targetRef{SectionID: "sec_1", ParentPath: []int{}, BlockIndex: 0, Kind: targetBlock}},
		{in: "sec_1.blocks[0].blocks[2]", want: targetRef{SectionID: "sec_1", ParentPath: []int{0}, BlockIndex: 2, Kind: targetBlock}},
		{in: "sec_1.blocks", want: targetRef{SectionID: "sec_1", BlockIndex: -1, Kind: targetContainer}},
		{in: "sec_1.blocks[1].blocks", want: targetRef{SectionID: "sec_1", ParentPath: []int{1}, BlockIndex: -1, Kind: targetContainer}},
		{in: "", wantErr: true},
		{in: ".blocks[0]", wantErr: true},
		{in: "sec_1.blocks[x]", wantErr: true},
		{in: "sec_1.blocks[-1]", wantErr: true},
		{in: "sec_1.blocks[0", wantErr: true},
		{in: "sec_1.blocks.blocks[0]", wantErr: true}, // segments after container suffix
		{in: "sec_1.blocksx", wantErr: true},
	}
	for _, tc := range cases {
		got, err := parseTarget(tc.in)
		if tc.wantErr {
			if err == nil {
				t.Errorf("parseTarget(%q): want error, got %+v", tc.in, got)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseTarget(%q): %v", tc.in, err)
			continue
		}
		if got.SectionID != tc.want.SectionID || got.BlockIndex != tc.want.BlockIndex || got.Kind != tc.want.Kind ||
			!equalIntSlice(got.ParentPath, tc.want.ParentPath) {
			t.Errorf("parseTarget(%q) = %+v, want %+v", tc.in, got, tc.want)
		}
	}
}

func equalIntSlice(a, b []int) bool {
	if len(a) != len(b) {
		return false
	}
	return len(a) == 0 || reflect.DeepEqual(a, b)
}

// The real dev-store sample (schemas_list_sample.json) must flatten cleanly.
func TestFlattenBlocks_RealSample(t *testing.T) {
	inner := loadSchemasListSample(t)
	page, fixed := splitSections(inner)
	if len(page) == 0 || len(fixed) == 0 {
		t.Fatalf("sample split: page=%d fixed=%d, want both non-empty", len(page), len(fixed))
	}
	hero := page[0]
	blocks, _ := hero["blocks"].([]any)
	rows := flattenBlocks("1638950411341", blocks)
	if len(rows) != 2 {
		t.Fatalf("hero_slideshow rows = %d, want 2", len(rows))
	}
	for i, r := range rows {
		if r.Type != "slide" {
			t.Errorf("row %d type = %q, want slide", i, r.Type)
		}
		if _, err := parseTarget(r.Target); err != nil {
			t.Errorf("row %d target %q does not parse: %v", i, r.Target, err)
		}
		if r.Settings == nil {
			t.Errorf("row %d settings missing", i)
		}
	}
}

// loadSchemasListSample loads the sanitized real schemas-list response
// captured from the dev store (see plan 01 DoD) and returns its inner
// {schemas, sections} payload.
func loadSchemasListSample(t *testing.T) map[string]any {
	t.Helper()
	_, self, _, _ := runtime.Caller(0)
	raw, err := os.ReadFile(filepath.Join(filepath.Dir(self), "testdata", "schemas_list_sample.json"))
	if err != nil {
		t.Fatalf("read sample: %v", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(raw, &resp); err != nil {
		t.Fatalf("unmarshal sample: %v", err)
	}
	inner := resp
	for i := 0; i < 2; i++ {
		if _, ok := inner["sections"]; ok {
			break
		}
		if d, ok := inner["data"].(map[string]any); ok {
			inner = d
		}
	}
	return inner
}
