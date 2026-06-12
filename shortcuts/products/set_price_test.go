package products

import (
	"testing"

	"shoplazza-cli-v2/shortcuts/common"
)

func TestSetPriceShortcut_ValidationFields(t *testing.T) {
	if setPriceShortcut.Service != "products" || setPriceShortcut.Command != "+set-price" {
		t.Errorf("identity: got %q/%q", setPriceShortcut.Service, setPriceShortcut.Command)
	}
	if setPriceShortcut.Execute == nil {
		t.Fatal("+set-price requires Execute (handles by-sku vs by-product-id branching)")
	}
	if err := common.ValidateShortcut(setPriceShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

func TestPickVariantIDFromList_ReturnsFirstMatching(t *testing.T) {
	resp := map[string]any{
		"variants": []any{
			map[string]any{"id": "v-1", "sku": "OTHER"},
			map[string]any{"id": "v-2", "sku": "SHIRT-M"},
		},
	}
	got, err := pickVariantIDForSKU(resp, "SHIRT-M")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "v-2" {
		t.Errorf("got %q want v-2", got)
	}
}

func TestPickVariantIDFromList_NoMatch(t *testing.T) {
	resp := map[string]any{"variants": []any{}}
	_, err := pickVariantIDForSKU(resp, "MISSING")
	if err == nil {
		t.Fatal("expected error when no variant matches")
	}
}
