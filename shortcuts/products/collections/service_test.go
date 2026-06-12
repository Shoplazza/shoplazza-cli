package collections

import (
	"strings"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestCollectionShortcuts_NonEmpty(t *testing.T) {
	ss := Shortcuts()
	if len(ss) == 0 {
		t.Error("Shortcuts() should return at least one shortcut")
	}
	for _, s := range ss {
		if err := common.ValidateShortcut(s); err != nil {
			t.Errorf("shortcut %q invalid: %v", s.Command, err)
		}
	}
}

func TestCollectionPlanCreate_Shape(t *testing.T) {
	body := map[string]any{"title": "Summer"}
	p := PlanCreate(body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/collections") {
		t.Errorf("Path: got %q want suffix /collections", p.Path)
	}
	b, _ := p.Body.(map[string]any)
	if b["title"] != "Summer" {
		t.Errorf("Body not propagated: %v", p.Body)
	}
}

func TestCollectionPlanBatchAssociate_Shape(t *testing.T) {
	body := map[string]any{"collects": []any{"p-1", "p-2"}}
	p := PlanBatchAssociate(body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/collects/batch") {
		t.Errorf("Path: got %q want suffix /collects/batch", p.Path)
	}
}

// ── extractCollectionID ───────────────────────────────────────────────────────

func TestExtractCollectionID_Valid(t *testing.T) {
	resp := map[string]any{"collection": map[string]any{"id": "col-123"}}
	if got := extractCollectionID(resp); got != "col-123" {
		t.Errorf("got %q want col-123", got)
	}
}

func TestExtractCollectionID_Missing(t *testing.T) {
	if got := extractCollectionID(map[string]any{"other": "val"}); got != "" {
		t.Errorf("missing collection: got %q want empty", got)
	}
}

func TestExtractCollectionID_NoID(t *testing.T) {
	resp := map[string]any{"collection": map[string]any{"name": "summer"}}
	if got := extractCollectionID(resp); got != "" {
		t.Errorf("no id field: got %q want empty", got)
	}
}
