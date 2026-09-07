package products

import (
	"testing"
)

var productCountFlags = map[string]string{
	"published": "string",
}

func TestProductCountPlan_DefaultsSuccess(t *testing.T) {
	in := newProductPlanInput(t, "count", productCountFlags, nil)
	_, err := countShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductCountPlan_PublishedNormalizedAndInvalid(t *testing.T) {
	in := newProductPlanInput(t, "count", productCountFlags, map[string]string{"published": "false"})
	p, err := countShortcut.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	if p.Query["published_status"] != "unpublished" {
		t.Errorf("--published false -> %v, want unpublished", p.Query["published_status"])
	}
	bad := newProductPlanInput(t, "count", productCountFlags, map[string]string{"published": "nope"})
	if _, err := countShortcut.Plan(bad); err == nil {
		t.Error("expected error for invalid --published")
	}
}
