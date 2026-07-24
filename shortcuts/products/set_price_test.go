package products

import (
	"errors"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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

func TestResolveSingleVariant_OneMatch(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "sku": "OTHER"},
		map[string]any{"id": "v-2", "sku": "SHIRT-M"},
	}}
	got, err := resolveSingleVariant(resp, "SHIRT-M")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "v-2" {
		t.Errorf("got %q want v-2", got)
	}
}

func TestResolveSingleVariant_NoMatch(t *testing.T) {
	resp := map[string]any{"variants": []any{}}
	if _, err := resolveSingleVariant(resp, "MISSING"); err == nil {
		t.Fatal("expected error when no variant matches")
	}
}

func TestResolveSingleVariant_MultiMatchRefusedWithCandidates(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "sku": "DUP"},
		map[string]any{"id": "v-2", "sku": "DUP"},
	}}
	_, err := resolveSingleVariant(resp, "DUP")
	if err == nil {
		t.Fatal("expected error on multi-match")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	// No new field: candidate ids go in the hint; the message stays a one-liner.
	if !strings.Contains(ee.Detail.Hint, "v-1") || !strings.Contains(ee.Detail.Hint, "v-2") {
		t.Errorf("hint should list candidate ids; got hint=%q", ee.Detail.Hint)
	}
	if strings.Contains(ee.Detail.Message, "\n") {
		t.Errorf("message should be a single line; got %q", ee.Detail.Message)
	}
}

func TestVariantSKU(t *testing.T) {
	resp := map[string]any{"variant": map[string]any{"id": "v-1", "sku": "ABC"}}
	if got := variantSKU(resp); got != "ABC" {
		t.Errorf("variantSKU = %q want ABC", got)
	}
	if got := variantSKU(map[string]any{}); got != "" {
		t.Errorf("variantSKU(empty) = %q want empty", got)
	}
}
