package products

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
)

var productCreateFlags = map[string]string{
	"title": "string", "price": "string", "image": "string",
	"compare-price": "string", "sku": "string", "stock": "int",
	"stock-policy": "string", "tags": "stringslice", "published": "bool",
	"collection-ids": "stringslice",
}

func TestProductCreatePlan_InvalidPriceErrors(t *testing.T) {
	in := shortcuttest.PlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "notanumber", "image": "http://img.example.com/x.jpg",
	})
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for non-numeric --price")
	}
}

func TestProductCreatePlan_NegativePriceErrors(t *testing.T) {
	in := shortcuttest.PlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "-5", "image": "http://img.example.com/x.jpg",
	})
	_, err := createShortcut.Plan(in)
	if err == nil || !strings.Contains(err.Error(), ">= 0") {
		t.Errorf("expected >=0 error for negative --price, got %v", err)
	}
}

func TestProductCreatePlan_ValidSuccess(t *testing.T) {
	in := shortcuttest.PlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "29.99", "image": "http://img.example.com/x.jpg",
	})
	_, err := createShortcut.Plan(in)
	if err != nil {
		t.Errorf("unexpected error: %v", err)
	}
}

func TestProductCreatePlan_InvalidComparePriceErrors(t *testing.T) {
	in := shortcuttest.PlanInput(t, "create", productCreateFlags, map[string]string{
		"title": "Shirt", "price": "29.99", "image": "http://img.example.com/x.jpg",
		"compare-price": "notanumber",
	})
	_, err := createShortcut.Plan(in)
	if err == nil {
		t.Error("expected error for non-numeric --compare-price")
	}
}
