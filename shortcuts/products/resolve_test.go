package products

import (
	"encoding/json"
	"errors"
	"fmt"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// ── resolveSingleVariant (SKU → variant id) ───────────────────────────────────

func TestResolveSingleVariant_OneMatch(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "sku": "OTHER"},
		map[string]any{"id": "v-2", "sku": "SHIRT-M"},
	}}
	got, err := resolveSingleVariant(resp, "SHIRT-M", true)
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "v-2" {
		t.Errorf("got %q want v-2", got)
	}
}

func TestResolveSingleVariant_NoMatch(t *testing.T) {
	resp := map[string]any{"variants": []any{}}
	if _, err := resolveSingleVariant(resp, "MISSING", true); err == nil {
		t.Fatal("expected error when no variant matches")
	}
}

func TestResolveSingleVariant_MultiMatchRefusedWithCandidates(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "sku": "DUP"},
		map[string]any{"id": "v-2", "sku": "DUP"},
	}}
	_, err := resolveSingleVariant(resp, "DUP", true)
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

// A hint must never name a flag the command the agent just ran does not have.
// +stock has no --all, so resolving on its behalf must not suggest one.
func TestResolveSingleVariant_MultiMatch_NoAllHintOmitsAllFlag(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "sku": "DUP"},
		map[string]any{"id": "v-2", "sku": "DUP"},
	}}
	_, err := resolveSingleVariant(resp, "DUP", false)
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if strings.Contains(ee.Detail.Hint, "--all") {
		t.Errorf("hint must not mention --all when the caller lacks it; got %q", ee.Detail.Hint)
	}
	if !strings.Contains(ee.Detail.Hint, "--variant-id") {
		t.Errorf("hint should still offer --variant-id; got %q", ee.Detail.Hint)
	}
}

// ── resolveOnlyVariant (product id → variant id) ──────────────────────────────

func TestResolveOnlyVariant_One(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-9", "sku": "ANY"},
	}}
	got, err := resolveOnlyVariant(resp, "p-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "v-9" {
		t.Errorf("got %q want v-9", got)
	}
}

func TestResolveOnlyVariant_Zero(t *testing.T) {
	if _, err := resolveOnlyVariant(map[string]any{"variants": []any{}}, "p-1"); err == nil {
		t.Fatal("expected error when the product has no variants")
	}
}

func TestResolveOnlyVariant_MultiRefusesWithLabelledCandidates(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": "v-1", "option1": "Red", "option2": "L"},
		map[string]any{"id": "v-2", "title": "Blue / M"},
	}}
	_, err := resolveOnlyVariant(resp, "p-1")
	if err == nil {
		t.Fatal("expected refusal on a multi-variant product")
	}
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	// Candidates carry a readable name so the caller can ask the user to pick
	// without a second lookup.
	if !strings.Contains(ee.Detail.Hint, "v-1 (Red / L)") {
		t.Errorf("hint should label option-based candidates; got %q", ee.Detail.Hint)
	}
	if !strings.Contains(ee.Detail.Hint, "v-2 (Blue / M)") {
		t.Errorf("hint should label title-based candidates; got %q", ee.Detail.Hint)
	}
}

// Large ids arrive as json.Number (client decodes with UseNumber), so the
// resolvers must go through asString rather than a bare string assertion.
func TestResolveOnlyVariant_NumericID(t *testing.T) {
	resp := map[string]any{"variants": []any{
		map[string]any{"id": json.Number("588599777604678400")},
	}}
	got, err := resolveOnlyVariant(resp, "p-1")
	if err != nil {
		t.Fatalf("unexpected err: %v", err)
	}
	if got != "588599777604678400" {
		t.Errorf("got %q want 588599777604678400", got)
	}
}

// ── variantTarget ─────────────────────────────────────────────────────────────

func TestVariantTarget_ResolvePlanRoutesBySelector(t *testing.T) {
	skuPlan := variantTarget{SKU: "TEE-RED-L"}.ResolvePlan()
	if !strings.HasSuffix(skuPlan.Path, "/products/sku/TEE-RED-L/variants") {
		t.Errorf("SKU should route to the sku-scoped list; got %q", skuPlan.Path)
	}
	prodPlan := variantTarget{ProductID: "p-1"}.ResolvePlan()
	if !strings.HasSuffix(prodPlan.Path, "/products/p-1/variants") {
		t.Errorf("product id should route to the product-scoped list; got %q", prodPlan.Path)
	}
}

func TestVariantTarget_NeedsResolve(t *testing.T) {
	if (variantTarget{VariantID: "v-1"}).NeedsResolve() {
		t.Error("a variant id needs no resolution")
	}
	if resolveNeeded := (variantTarget{ProductID: "p-1"}).NeedsResolve(); !resolveNeeded {
		t.Error("a product id needs resolution")
	}
}

// ── hintIfDirectVariantID ─────────────────────────────────────────────────────

func TestHintIfDirectVariantID_404GetsRecoveryHint(t *testing.T) {
	err := &client.HTTPError{StatusCode: 404, Body: `{"message":"No variants found"}`}
	got := variantTarget{VariantID: "p-looks-like-a-product"}.hintIfDirectVariantID(err)
	var ee *output.ExitError
	if !errors.As(got, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", got)
	}
	if !strings.Contains(ee.Detail.Hint, "--product-id") {
		t.Errorf("hint should point at --product-id; got %q", ee.Detail.Hint)
	}
}

func TestHintIfDirectVariantID_EmptyInventoryLookupGetsHint(t *testing.T) {
	_, err := extractInventoryItemID(map[string]any{"variant_inventory_items": []any{}})
	got := variantTarget{VariantID: "p-looks-like-a-product"}.hintIfDirectVariantID(err)
	var ee *output.ExitError
	if !errors.As(got, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", got)
	}
	if ee.Code != output.ExitValidation {
		t.Errorf("a mistyped id is user input, not a CLI bug: exit %d want %d", ee.Code, output.ExitValidation)
	}
	if !strings.Contains(ee.Detail.Hint, "--product-id") {
		t.Errorf("hint should point at --product-id; got %q", ee.Detail.Hint)
	}
}

// After --sku / --product-id resolution the variant id is ours, so these same
// failures mean something else and must not carry the misdirecting hint.
func TestHintIfDirectVariantID_SilentForResolvedTargets(t *testing.T) {
	err := &client.HTTPError{StatusCode: 404, Body: "{}"}
	if got := (variantTarget{ProductID: "p-1"}).hintIfDirectVariantID(err); got != error(err) {
		t.Errorf("a resolved target must pass the error through unchanged; got %v", got)
	}
	if got := (variantTarget{SKU: "S"}).hintIfDirectVariantID(err); got != error(err) {
		t.Errorf("a resolved target must pass the error through unchanged; got %v", got)
	}
}

func TestStepRef(t *testing.T) {
	if got := stepRef(0); got != "<resolved-from-step-0>" {
		t.Errorf("stepRef(0) = %q", got)
	}
	if got := stepRef(2); got != "<resolved-from-step-2>" {
		t.Errorf("stepRef(2) = %q", got)
	}
}

// A 45-variant product must not dump 45 ids into one hint line. The cap is
// stated, not silent, and it names the command that lists the rest.
func TestResolveOnlyVariant_ManyVariantsTruncatesAndSaysSo(t *testing.T) {
	rows := make([]any, 0, 45)
	for i := range 45 {
		rows = append(rows, map[string]any{"id": fmt.Sprintf("v-%02d", i), "option1": "Black"})
	}
	_, err := resolveOnlyVariant(map[string]any{"variants": rows}, "p-1")
	var ee *output.ExitError
	if !errors.As(err, &ee) {
		t.Fatalf("expected *output.ExitError, got %T", err)
	}
	if strings.Contains(ee.Detail.Hint, "v-44") {
		t.Error("hint should be capped, not list every candidate")
	}
	if !strings.Contains(ee.Detail.Hint, "and 35 more") {
		t.Errorf("truncation must be stated; got %q", ee.Detail.Hint)
	}
	if !strings.Contains(ee.Detail.Hint, "products variants list") {
		t.Errorf("truncated hint must name the command listing the rest; got %q", ee.Detail.Hint)
	}
	// The count in the message stays exact even though the list is capped.
	if !strings.Contains(ee.Detail.Message, "45 variants") {
		t.Errorf("message should report the true count; got %q", ee.Detail.Message)
	}
}
