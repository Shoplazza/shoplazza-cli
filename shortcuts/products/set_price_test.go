package products

import (
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestSetPriceShortcut_ValidationFields(t *testing.T) {
	if setPriceShortcut.Service != "products" || setPriceShortcut.Command != "+set-price" {
		t.Errorf("identity: got %q/%q", setPriceShortcut.Service, setPriceShortcut.Command)
	}
	if setPriceShortcut.Execute == nil {
		t.Fatal("+set-price requires Execute (handles variant-id / sku / --all branching)")
	}
	if err := common.ValidateShortcut(setPriceShortcut); err != nil {
		t.Errorf("validate: %v", err)
	}
}

// Identifier-resolution tests live in resolve_test.go alongside the code.

func TestVariantSKU(t *testing.T) {
	resp := map[string]any{"variant": map[string]any{"id": "v-1", "sku": "ABC"}}
	if got := variantSKU(resp); got != "ABC" {
		t.Errorf("variantSKU = %q want ABC", got)
	}
	if got := variantSKU(map[string]any{}); got != "" {
		t.Errorf("variantSKU(empty) = %q want empty", got)
	}
}
