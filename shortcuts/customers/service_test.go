package customers

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

func TestCustomerPlanList_Shape(t *testing.T) {
	p := PlanList(map[string]any{"limit": 20})
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/customers") {
		t.Errorf("Path: got %q want suffix /customers", p.Path)
	}
	if p.Query["limit"] != 20 {
		t.Errorf("Query not propagated: %v", p.Query)
	}
}

func TestCustomerSearchPlan_Paging(t *testing.T) {
	flags := map[string]string{"email": "string", "phone": "string", "since": "string", "until": "string", "page-limit": "int", "cursor": "string"}
	p, err := searchShortcut.Plan(shortcuttest.PlanInput(t, "search", flags, map[string]string{"page-limit": "5", "cursor": "c-2"}))
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if p.Query["cursor"] != "c-2" || p.Query["page_size"] != 5 {
		t.Errorf("--cursor/--page-limit must reach cursor/page_size, got %v / %v", p.Query["cursor"], p.Query["page_size"])
	}
}

func TestCustomerPlanList_NilQuery(t *testing.T) {
	p := PlanList(nil)
	if p.Method != "GET" || p.Query != nil {
		t.Errorf("PlanList(nil): Method=%q Query=%v", p.Method, p.Query)
	}
}

func TestCustomerPlanCount_Shape(t *testing.T) {
	p := PlanCount(map[string]any{"email": "a@b.com"})
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/customers/count") {
		t.Errorf("Path: got %q want suffix /customers/count", p.Path)
	}
	if p.Query["email"] != "a@b.com" {
		t.Errorf("Query not propagated: %v", p.Query)
	}
}

func TestCustomerPlanCreate_Shape(t *testing.T) {
	body := map[string]any{"email": "a@b.com"}
	p := PlanCreate(body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/customers") {
		t.Errorf("Path: got %q want suffix /customers", p.Path)
	}
	b, _ := p.Body.(map[string]any)
	if b["email"] != "a@b.com" {
		t.Errorf("Body not propagated: %v", p.Body)
	}
}
