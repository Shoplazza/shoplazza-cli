package shop

import (
	"strings"
	"testing"
)

func TestPlanFileUpload_Shape(t *testing.T) {
	p := PlanFileUpload([]string{"https://example.com/img.jpg"})
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
	if len(b) != 1 {
		t.Errorf("Body: got %v want only original_source_list", b)
	}
	urls, _ := b["original_source_list"].([]string)
	if len(urls) != 1 || urls[0] != "https://example.com/img.jpg" {
		t.Errorf("original_source_list: %v", b["original_source_list"])
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
