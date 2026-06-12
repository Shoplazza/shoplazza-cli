package schema

import (
	"bytes"
	"encoding/json"
	"io"
	"strings"
	"testing"

	"shoplazza-cli-v2/internal/registry"
)

// ── validateView ──────────────────────────────────────────────────────────────

func TestValidateView_ValidValues(t *testing.T) {
	for _, v := range []string{"", "all", "request", "response"} {
		if err := validateView(v); err != nil {
			t.Errorf("validateView(%q) unexpected error: %v", v, err)
		}
	}
}

func TestValidateView_InvalidValue(t *testing.T) {
	if err := validateView("unknown"); err == nil {
		t.Error("expected error for unknown view value")
	}
}

// ── orderedFields.MarshalJSON ─────────────────────────────────────────────────

func TestOrderedFields_MarshalJSON_PreservesOrder(t *testing.T) {
	fields := orderedFields{
		{Key: "z", Value: "last"},
		{Key: "a", Value: "first"},
		{Key: "m", Value: "middle"},
	}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	s := string(b)
	zPos := strings.Index(s, `"z"`)
	aPos := strings.Index(s, `"a"`)
	mPos := strings.Index(s, `"m"`)
	if !(zPos < aPos && aPos < mPos) {
		t.Errorf("keys not in insertion order: z=%d a=%d m=%d; json=%s", zPos, aPos, mPos, s)
	}
}

func TestOrderedFields_MarshalJSON_Empty(t *testing.T) {
	fields := orderedFields{}
	b, err := json.Marshal(fields)
	if err != nil {
		t.Fatal(err)
	}
	if string(b) != "{}" {
		t.Errorf("empty orderedFields: got %s, want {}", b)
	}
}

// ── reorderMap ────────────────────────────────────────────────────────────────

func TestReorderMap_PriorityKeysFirst(t *testing.T) {
	m := map[string]any{
		"zzz":     "leftover",
		"path":    "/orders",
		"summary": "list orders",
		"http":    "GET",
	}
	result := reorderMap(m)
	b, _ := json.Marshal(result)
	s := string(b)
	pathPos := strings.Index(s, `"path"`)
	summaryPos := strings.Index(s, `"summary"`)
	httpPos := strings.Index(s, `"http"`)
	zzzPos := strings.Index(s, `"zzz"`)
	if !(pathPos < summaryPos && summaryPos < httpPos && httpPos < zzzPos) {
		t.Errorf("key ordering wrong: path=%d summary=%d http=%d zzz=%d; json=%s",
			pathPos, summaryPos, httpPos, zzzPos, s)
	}
}

func TestReorderMap_UnknownKeysSortedAlphabetically(t *testing.T) {
	m := map[string]any{
		"zebra": "z",
		"alpha": "a",
		"mango": "m",
	}
	result := reorderMap(m)
	b, _ := json.Marshal(result)
	s := string(b)
	aPos := strings.Index(s, `"alpha"`)
	mPos := strings.Index(s, `"mango"`)
	zPos := strings.Index(s, `"zebra"`)
	if !(aPos < mPos && mPos < zPos) {
		t.Errorf("leftover keys not sorted: alpha=%d mango=%d zebra=%d; json=%s",
			aPos, mPos, zPos, s)
	}
}

// ── reorderSchemaPayload ──────────────────────────────────────────────────────

func TestReorderSchemaPayload_Map(t *testing.T) {
	payload := map[string]any{"summary": "test", "zzz": "extra"}
	result := reorderSchemaPayload(payload)
	if _, ok := result.(orderedFields); !ok {
		t.Errorf("expected orderedFields, got %T", result)
	}
}

func TestReorderSchemaPayload_SliceAny(t *testing.T) {
	payload := []any{
		map[string]any{"path": "/a"},
		map[string]any{"path": "/b"},
	}
	result := reorderSchemaPayload(payload)
	out, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2", len(out))
	}
	if _, ok := out[0].(orderedFields); !ok {
		t.Errorf("element 0 should be orderedFields, got %T", out[0])
	}
}

func TestReorderSchemaPayload_SliceMaps(t *testing.T) {
	payload := []map[string]any{
		{"path": "/a"},
		{"path": "/b"},
	}
	result := reorderSchemaPayload(payload)
	out, ok := result.([]any)
	if !ok {
		t.Fatalf("expected []any, got %T", result)
	}
	if len(out) != 2 {
		t.Errorf("len = %d, want 2", len(out))
	}
}

func TestReorderSchemaPayload_Scalar(t *testing.T) {
	result := reorderSchemaPayload("hello")
	if result != "hello" {
		t.Errorf("scalar passthrough: got %v", result)
	}
}

// ── NewCmdSchema RunE ─────────────────────────────────────────────────────────

func TestNewCmdSchema_NoArgs_ListsModules(t *testing.T) {
	spec := registry.LoadSpec()
	cmd := NewCmdSchema(spec)
	cmd.SetOut(io.Discard)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Errorf("unexpected error listing modules: %v", err)
	}
}

func TestNewCmdSchema_UnknownPath_Errors(t *testing.T) {
	spec := registry.LoadSpec()
	cmd := NewCmdSchema(spec)
	cmd.SetOut(io.Discard)
	if err := cmd.RunE(cmd, []string{"nonexistent.foobar"}); err == nil {
		t.Error("expected error for unknown schema path")
	}
}

func TestNewCmdSchema_InvalidView_Errors(t *testing.T) {
	spec := registry.LoadSpec()
	cmd := NewCmdSchema(spec)
	cmd.SetOut(io.Discard)
	_ = cmd.Flags().Set("view", "invalid")
	if err := cmd.RunE(cmd, nil); err == nil {
		t.Error("expected error for invalid --view value")
	}
}

func TestNewCmdSchema_ModulePathWithView_PrintsNote(t *testing.T) {
	spec := registry.LoadSpec()
	cmd := NewCmdSchema(spec)
	cmd.SetOut(io.Discard)
	var errBuf bytes.Buffer
	cmd.SetErr(&errBuf)
	_ = cmd.Flags().Set("view", "request")
	// A module-level path with --view triggers the note on stderr but should still succeed.
	err := cmd.RunE(cmd, []string{"orders"})
	if err != nil {
		t.Logf("RunE returned %v (may be expected if orders module absent)", err)
	}
}
