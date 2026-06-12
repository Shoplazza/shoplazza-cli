package products

import (
	"regexp"
	"strings"
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestProductShortcuts_NonEmpty(t *testing.T) {
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

func TestProductPlanList_Shape(t *testing.T) {
	p := PlanList(map[string]any{"limit": 10})
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/products") {
		t.Errorf("PlanList: got Method=%q Path=%q", p.Method, p.Path)
	}
	if p.Query["limit"] != 10 {
		t.Errorf("Query not propagated: %v", p.Query)
	}
}

func TestProductPlanCount_Shape(t *testing.T) {
	p := PlanCount(nil)
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/products/count") {
		t.Errorf("PlanCount: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanUpdate_Shape(t *testing.T) {
	body := map[string]any{"title": "T"}
	p := PlanUpdate("p-1", body)
	if p.Method != "PUT" || !strings.HasSuffix(p.Path, "/products/p-1") {
		t.Errorf("PlanUpdate: got Method=%q Path=%q", p.Method, p.Path)
	}
	b, _ := p.Body.(map[string]any)
	if b["title"] != "T" {
		t.Errorf("Body not propagated: %v", p.Body)
	}
}

func TestProductPlanGet_Shape(t *testing.T) {
	p := PlanGet("p-2")
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/products/p-2") {
		t.Errorf("PlanGet: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanCreate_Shape(t *testing.T) {
	body := map[string]any{"title": "New"}
	p := PlanCreate(body)
	if p.Method != "POST" || !strings.HasSuffix(p.Path, "/products") {
		t.Errorf("PlanCreate: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanUpdateVariantBySKU_Shape(t *testing.T) {
	body := map[string]any{"price": "9.99"}
	p := PlanUpdateVariantBySKU("SKU-1", body)
	if p.Method != "PUT" || !strings.HasSuffix(p.Path, "/variants/sku/SKU-1") {
		t.Errorf("PlanUpdateVariantBySKU: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanUpdateVariant_Shape(t *testing.T) {
	body := map[string]any{"price": "5.00"}
	p := PlanUpdateVariant("v-3", body)
	if p.Method != "PUT" || !strings.HasSuffix(p.Path, "/variants/v-3") {
		t.Errorf("PlanUpdateVariant: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanListVariantsByProductSKU_Shape(t *testing.T) {
	p := PlanListVariantsByProductSKU("p-1", "SKU-X")
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/products/p-1/variants") {
		t.Errorf("PlanListVariantsByProductSKU: got Method=%q Path=%q", p.Method, p.Path)
	}
	skus, _ := p.Query["sku"].(string)
	if skus != "SKU-X" {
		t.Errorf("sku query param: got %v want SKU-X", p.Query["sku"])
	}
}

func TestProductPlanInventoryItemForVariant_Shape(t *testing.T) {
	p := PlanInventoryItemForVariant("v-1")
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/inventory_items/variant") {
		t.Errorf("PlanInventoryItemForVariant: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanDefaultLocation_Shape(t *testing.T) {
	p := PlanDefaultLocation()
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/locations/default") {
		t.Errorf("PlanDefaultLocation: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanSetInventoryLevel_Shape(t *testing.T) {
	body := map[string]any{"stock": 10}
	p := PlanSetInventoryLevel(body)
	if p.Method != "POST" || !strings.HasSuffix(p.Path, "/inventory_levels/set") {
		t.Errorf("PlanSetInventoryLevel: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanAdjustInventoryLevel_Shape(t *testing.T) {
	body := map[string]any{"stock_adjustment": 5}
	p := PlanAdjustInventoryLevel(body)
	if p.Method != "PUT" || !strings.HasSuffix(p.Path, "/inventory_levels") {
		t.Errorf("PlanAdjustInventoryLevel: got Method=%q Path=%q", p.Method, p.Path)
	}
}

func TestProductPlanGetInventoryLevel_Shape(t *testing.T) {
	p := PlanGetInventoryLevel("ii-1", "loc-1")
	if p.Method != "GET" || !strings.HasSuffix(p.Path, "/inventory_levels") {
		t.Errorf("PlanGetInventoryLevel: got Method=%q Path=%q", p.Method, p.Path)
	}
	ids, _ := p.Query["inventory_item_ids"].([]string)
	if len(ids) != 1 || ids[0] != "ii-1" {
		t.Errorf("inventory_item_ids: got %v want [ii-1]", ids)
	}
	locs, _ := p.Query["location_ids"].([]string)
	if len(locs) != 1 || locs[0] != "loc-1" {
		t.Errorf("location_ids: got %v want [loc-1]", locs)
	}
}

// ── generateUniqueToken ───────────────────────────────────────────────────────

var uuidRE = regexp.MustCompile(`^[0-9a-f]{8}-[0-9a-f]{4}-4[0-9a-f]{3}-[89ab][0-9a-f]{3}-[0-9a-f]{12}$`)

func TestGenerateUniqueToken_IsUUIDv4(t *testing.T) {
	got := generateUniqueToken("create")
	if !uuidRE.MatchString(got) {
		t.Errorf("generateUniqueToken = %q, not a valid UUIDv4", got)
	}
}

func TestGenerateUniqueToken_Unique(t *testing.T) {
	seen := map[string]bool{}
	for i := 0; i < 50; i++ {
		tok := generateUniqueToken("create")
		if seen[tok] {
			t.Fatalf("generateUniqueToken produced duplicate: %q", tok)
		}
		seen[tok] = true
	}
}
