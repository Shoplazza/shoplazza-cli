package products

import (
	"testing"

	"github.com/spf13/cobra"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// newProductPlanInput builds a PlanInput via a cobra command.
func newProductPlanInput(t *testing.T, tool string, flags map[string]string, values map[string]string) common.PlanInput {
	t.Helper()
	cmd := &cobra.Command{Use: "test", RunE: func(*cobra.Command, []string) error { return nil }}
	cmd.SilenceErrors = true
	cmd.SilenceUsage = true
	for name, typ := range flags {
		switch typ {
		case "string":
			cmd.Flags().String(name, "", "")
		case "int":
			cmd.Flags().Int(name, 0, "")
		case "bool":
			cmd.Flags().Bool(name, false, "")
		case "stringslice":
			cmd.Flags().StringSlice(name, nil, "")
		}
	}
	var args []string
	for name, val := range values {
		args = append(args, "--"+name+"="+val)
	}
	cmd.SetArgs(args)
	if err := cmd.Execute(); err != nil {
		t.Fatalf("cmd.Execute: %v", err)
	}
	return common.PlanInput{Tool: tool, Flags: common.NewCobraFlagSet(cmd)}
}

// ── searchShortcut.Plan ───────────────────────────────────────────────────────

var productSearchFlags = map[string]string{
	"keyword": "string", "published": "string", "vendor": "string",
	"collection-id": "string",
	"page-limit":    "int", "fields": "stringslice",
}

func TestProductSearchPlan_DefaultsSuccess(t *testing.T) {
	in := newProductPlanInput(t, "search", productSearchFlags, nil)
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_WithKeywordSuccess(t *testing.T) {
	in := newProductPlanInput(t, "search", productSearchFlags, map[string]string{
		"keyword": "shirt", "page-limit": "10",
	})
	_, err := searchShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductSearchPlan_VendorMapsToVendorsArray(t *testing.T) {
	in := newProductPlanInput(t, "search", productSearchFlags, map[string]string{"vendor": "Acme"})
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
		pin := newProductPlanInput(t, "search", productSearchFlags, map[string]string{"published": in})
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
	in := newProductPlanInput(t, "search", productSearchFlags, map[string]string{"published": "yes"})
	if _, err := searchShortcut.Plan(in); err == nil {
		t.Error("expected error for an invalid --published value")
	}
}

// ── countShortcut.Plan ────────────────────────────────────────────────────────

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

// ── publishShortcutValue.Plan / unpublishShortcutValue.Plan ───────────────────

var productIDFlags = map[string]string{"id": "string"}

func TestPublishShortcutPlan_Success(t *testing.T) {
	in := newProductPlanInput(t, "publish", productIDFlags, map[string]string{"id": "prod-1"})
	_, err := publishShortcutValue.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestUnpublishShortcutPlan_Success(t *testing.T) {
	in := newProductPlanInput(t, "unpublish", productIDFlags, map[string]string{"id": "prod-1"})
	_, err := unpublishShortcutValue.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

// ── createShortcut.Plan ───────────────────────────────────────────────────────

var productCreateFlags = map[string]string{
	"title": "string", "price": "string", "image": "string",
	"compare-price": "string", "sku": "string", "stock": "int",
	"stock-policy": "string", "tags": "stringslice", "published": "bool",
	"collection-ids": "stringslice",
}

func TestProductCreatePlan_InvalidPriceErrors(t *testing.T) {
	in := newProductPlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "notanumber", "image": "http://img.example.com/x.jpg",
	})
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for non-numeric --price")
	}
}

func TestProductCreatePlan_ValidSuccess(t *testing.T) {
	in := newProductPlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "29.99", "image": "http://img.example.com/x.jpg",
	})
	_, err := createShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductCreatePlan_InvalidComparePriceErrors(t *testing.T) {
	in := newProductPlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "29.99", "image": "http://img.example.com/x.jpg",
		"compare-price": "notanumber",
	})
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for non-numeric --compare-price")
	}
}
