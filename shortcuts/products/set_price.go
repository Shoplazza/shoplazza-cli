package products

import (
	"context"
	"strconv"
	"strings"

	"shoplazza-cli-v2/internal/output"
	"shoplazza-cli-v2/shortcuts/common"
)

var setPriceShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-price",
	Use:     "+set-price --sku <sku> --price <n> [--product-id <id>]",
	Short:   "Set a variant's price by SKU",
	Flags: []common.Flag{
		{Name: "sku", Type: common.FlagString, Required: true, Description: "Variant SKU (required)."},
		{Name: "price", Type: common.FlagString, Required: true, Description: "New price (required, e.g., '24.99')."},
		{Name: "compare-price", Type: common.FlagString, Description: "New compare-at price."},
		{Name: "product-id", Type: common.FlagString, Description: "Product ID (for SKU disambiguation when same SKU appears across products)."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		sku := strings.TrimSpace(in.Flags.GetString("sku"))
		priceStr := in.Flags.GetString("price")
		comparePrice := in.Flags.GetString("compare-price")
		productID := strings.TrimSpace(in.Flags.GetString("product-id"))

		price, err := strconv.ParseFloat(priceStr, 64)
		if err != nil {
			return common.ExecResult{}, output.ErrValidation("--price must be a number, got %q", priceStr)
		}
		variantBody := map[string]any{"price": price}
		if comparePrice != "" {
			cpf, err := strconv.ParseFloat(comparePrice, 64)
			if err != nil {
				return common.ExecResult{}, output.ErrValidation("--compare-price must be a number, got %q", comparePrice)
			}
			variantBody["compare_at_price"] = cpf
		}
		body := map[string]any{"variant": variantBody}

		if productID == "" {
			// Single-step path: PUT /variants/sku/{sku}
			plan := PlanUpdateVariantBySKU(sku, body)
			if in.DryRun {
				return common.ExecResult{Plans: []common.PlannedRequest{plan}}, nil
			}
			resp, err := common.Send(ctx, in.Client, plan)
			if err != nil {
				return common.ExecResult{}, err
			}
			return common.ExecResult{Body: resp}, nil
		}

		// Two-step path: GET /products/{pid}/variants?sku=… → PUT /variants/{vid}
		listPlan := PlanListVariantsByProductSKU(productID, sku)
		if in.DryRun {
			// Placeholder variant id for the dry-run preview.
			updatePlan := PlanUpdateVariant("<resolved-from-step-0>", body)
			return common.ExecResult{Plans: []common.PlannedRequest{listPlan, updatePlan}}, nil
		}
		resp, err := common.Send(ctx, in.Client, listPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		variantID, err := pickVariantIDForSKU(resp, sku)
		if err != nil {
			return common.ExecResult{}, err
		}
		updateResp, err := common.Send(ctx, in.Client, PlanUpdateVariant(variantID, body))
		if err != nil {
			return common.ExecResult{}, err
		}
		return common.ExecResult{Body: updateResp}, nil
	},
}

func pickVariantIDForSKU(resp map[string]any, sku string) (string, error) {
	raw, ok := resp["variants"].([]any)
	if !ok {
		return "", output.ErrInternal("variant list response missing 'variants' array")
	}
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if id, _ := m["id"].(string); id != "" {
			if s, _ := m["sku"].(string); s == sku {
				return id, nil
			}
		}
	}
	return "", output.ErrValidation("no variant with SKU %q under the specified product", sku)
}
