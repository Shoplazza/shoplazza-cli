package discounts

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

func TestDiscountShortcuts_NonEmpty(t *testing.T) {
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

func TestPlanList_Shape(t *testing.T) {
	p := PlanList(map[string]any{"limit": 10})
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/discounts") {
		t.Errorf("PlanList: Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestPlanCreateAutomatic_Shape(t *testing.T) {
	body := map[string]any{"discount": map[string]any{"discount_type": "rebate_cta_otr"}}
	p := PlanCreateAutomatic(body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/discounts/automatic") {
		t.Errorf("Path: got %q want suffix /discounts/automatic", p.Path)
	}
	b, _ := p.Body.(map[string]any)
	if b["discount"] == nil {
		t.Error("Body not propagated")
	}
}

func TestPlanCreateNonAutomatic_Shape(t *testing.T) {
	body := map[string]any{"discount": map[string]any{"discount_type": "code_percent"}}
	p := PlanCreateNonAutomatic(body)
	if p.Method != "POST" {
		t.Errorf("Method: got %q want POST", p.Method)
	}
	if !strings.HasSuffix(p.Path, "/discounts/non-automatic") {
		t.Errorf("Path: got %q want suffix /discounts/non-automatic", p.Path)
	}
}
