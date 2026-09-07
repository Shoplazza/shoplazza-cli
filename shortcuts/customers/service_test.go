package customers

import (
	"strings"
	"testing"
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

func TestCustomerPlanList_NilQuery(t *testing.T) {
	p := PlanList(nil)
	if p.Method != "GET" || p.Query != nil {
		t.Errorf("PlanList(nil): Method=%q Query=%v", p.Method, p.Query)
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
