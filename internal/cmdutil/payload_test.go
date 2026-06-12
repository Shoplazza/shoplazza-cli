package cmdutil_test

import (
	"testing"

	"shoplazza-cli-v2/internal/cmdutil"
)

func TestEnsureObject_CreatesNew(t *testing.T) {
	target := map[string]any{}
	child := cmdutil.EnsureObject(target, "meta")
	child["x"] = 1
	if target["meta"] == nil {
		t.Error("EnsureObject should set target key")
	}
	saved, _ := target["meta"].(map[string]any)
	if saved["x"] != 1 {
		t.Errorf("saved[x] = %v, want 1", saved["x"])
	}
}

func TestEnsureObject_ReturnsExisting(t *testing.T) {
	existing := map[string]any{"already": "here"}
	target := map[string]any{"meta": existing}
	got := cmdutil.EnsureObject(target, "meta")
	if got["already"] != "here" {
		t.Errorf("EnsureObject should return existing child, got %v", got)
	}
}

func TestEnsureObject_WrongType_CreatesNew(t *testing.T) {
	target := map[string]any{"meta": "not-a-map"}
	child := cmdutil.EnsureObject(target, "meta")
	if child == nil {
		t.Error("EnsureObject should return non-nil child")
	}
}

func TestAddString_NonEmpty(t *testing.T) {
	target := map[string]any{}
	cmdutil.AddString(target, "title", "summer sale")
	if target["title"] != "summer sale" {
		t.Errorf("title = %v, want 'summer sale'", target["title"])
	}
}

func TestAddString_Empty_Skipped(t *testing.T) {
	target := map[string]any{}
	cmdutil.AddString(target, "title", "")
	if _, ok := target["title"]; ok {
		t.Error("empty string should not be inserted")
	}
}

func TestAddString_Whitespace_Skipped(t *testing.T) {
	target := map[string]any{}
	cmdutil.AddString(target, "title", "   ")
	if _, ok := target["title"]; ok {
		t.Error("whitespace-only string should not be inserted")
	}
}

func TestAddSlice_NonEmpty(t *testing.T) {
	target := map[string]any{}
	cmdutil.AddSlice(target, "ids", []string{"a", "b"})
	vals, _ := target["ids"].([]string)
	if len(vals) != 2 {
		t.Errorf("ids len = %d, want 2", len(vals))
	}
}

func TestAddSlice_Empty_Skipped(t *testing.T) {
	target := map[string]any{}
	cmdutil.AddSlice(target, "ids", nil)
	if _, ok := target["ids"]; ok {
		t.Error("nil slice should not be inserted")
	}
	cmdutil.AddSlice(target, "ids", []string{})
	if _, ok := target["ids"]; ok {
		t.Error("empty slice should not be inserted")
	}
}
