package shop

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

func TestShopShortcuts_NonEmpty(t *testing.T) {
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

func TestPlanFileUpload_Shape(t *testing.T) {
	p := PlanFileUpload([]string{"https://example.com/img.jpg"}, "images")
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/file") {
		t.Errorf("Path: got %q want suffix /file", p.Path)
	}
	b, ok := p.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body not map[string]any: %T", p.Body)
	}
	if b["folder"] != "images" {
		t.Errorf("folder: got %v want images", b["folder"])
	}
	urls, _ := b["original_source_list"].([]string)
	if len(urls) != 1 || urls[0] != "https://example.com/img.jpg" {
		t.Errorf("original_source_list: %v", b["original_source_list"])
	}
}

func TestPlanFileUpload_NoFolder(t *testing.T) {
	p := PlanFileUpload([]string{"https://example.com/img.jpg"}, "")
	b, ok := p.Body.(map[string]any)
	if !ok {
		t.Fatalf("Body not map[string]any: %T", p.Body)
	}
	if _, exists := b["folder"]; exists {
		t.Error("folder key should be absent when empty")
	}
}

func TestPlanFileTask_Shape(t *testing.T) {
	p := PlanFileTask("task-abc")
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/task/task-abc") {
		t.Errorf("Path: got %q want suffix /task/task-abc", p.Path)
	}
}
