package products

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var productSearchFlags = map[string]string{
	"keyword": "string", "published": "string", "vendor": "string",
	"collection-id": "string",
	"page-limit":    "int", "fields": "stringslice",
}

func TestProductSearchPlan_DefaultsSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_WithKeywordSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{
		"keyword": "shirt", "page-limit": "10",
	})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_VendorMapsToVendorsArray(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"vendor": "Acme"})
	p, err := searchShortcut.Plan(in)
	if err != nil {
		t.Fatal(err)
	}
	vs, _ := p.Query["vendors"].([]string)
	if len(vs) != 1 || vs[0] != "Acme" {
		t.Errorf("vendor should map to vendors=[Acme]; got query=%v", p.Query)
	}
	if _, ok := p.Query["vendor"]; ok {
		t.Error("must not send the invalid singular `vendor` param")
	}
}

func TestProductSearchPlan_PublishedNormalized(t *testing.T) {
	cases := map[string]string{"true": "published", "false": "unpublished", "any": "any", "published": "published"}
	for in, want := range cases {
		pin := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"published": in})
		p, err := searchShortcut.Plan(pin)
		if err != nil {
			t.Fatalf("--published %q: %v", in, err)
		}
		if got := p.Query["published_status"]; got != want {
			t.Errorf("--published %q -> published_status=%v, want %q", in, got, want)
		}
	}
}

func TestProductSearchPlan_PublishedInvalidErrors(t *testing.T) {
	in := shortcuttest.PlanInput(t, "search", productSearchFlags, map[string]string{"published": "yes"})
	if _, err := searchShortcut.Plan(in); err == nil {
		t.Error("expected error for an invalid --published value")
	}
}
