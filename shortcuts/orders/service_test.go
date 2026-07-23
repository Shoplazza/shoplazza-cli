package orders

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

func TestPlanList_Shape(t *testing.T) {
	q := map[string]any{"status": "open"}
	p := PlanList(q)
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders") {
		t.Errorf("Path: got %q want suffix /orders", p.Path)
	}
	if p.Query["status"] != "open" {
		t.Errorf("Query not propagated: %v", p.Query)
	}
}

func TestPlanGet_Shape(t *testing.T) {
	p := PlanGet("ord-42")
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders/ord-42") {
		t.Errorf("Path: got %q want suffix /orders/ord-42", p.Path)
	}
}

func TestPlanCount_Shape(t *testing.T) {
	p := PlanCount(nil)
	if p.Method != "GET" {
		t.Errorf("Method: got %q want GET", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders/count") {
		t.Errorf("Path: got %q want suffix /orders/count", p.Path)
	}
}

func TestPlanCreateFulfillment_Shape(t *testing.T) {
	body := map[string]any{"tracking_number": "T1"}
	p := PlanCreateFulfillment("ord-1", body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders/ord-1/fulfillments") {
		t.Errorf("Path: got %q want suffix /orders/ord-1/fulfillments", p.Path)
	}
	b, _ := p.Body.(map[string]any)
	if b["tracking_number"] != "T1" {
		t.Errorf("Body not propagated: %v", p.Body)
	}
}

func TestPlanUpdateFulfillment_Shape(t *testing.T) {
	body := map[string]any{"tracking_number": "T2"}
	p := PlanUpdateFulfillment("ord-1", "ful-2", body)
	if p.Method != "PUT" {
		t.Errorf("Method: got %q want PUT", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders/ord-1/fulfillments/ful-2") {
		t.Errorf("Path: got %q want suffix /orders/ord-1/fulfillments/ful-2", p.Path)
	}
}

func TestPlanRefund_Shape(t *testing.T) {
	body := map[string]any{"refund_total": "10.00"}
	p := PlanRefund("ord-1", body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/orders/ord-1/refund") {
		t.Errorf("Path: got %q want suffix /orders/ord-1/refund", p.Path)
	}
}

func TestPlanList_IsPlannedRequest(t *testing.T) {
	var _ common.PlannedRequest = PlanList(nil)
}

func TestOrderShortcuts_NonEmpty(t *testing.T) {
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
