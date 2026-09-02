package products

import (
	"context"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var setPriceShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-price",
	Use:     "+set-price (--variant-id <id> | --sku <sku> [--all] | --product-id <id>) --price <n> [--compare-price <n>]",
	Short:   "Set a variant's price by variant ID, SKU, or product ID",
	Flags: []common.Flag{
		{Name: "variant-id", Type: common.FlagString, Description: "Variant ID — the unique, exact target."},
		{Name: "sku", Type: common.FlagString, Description: "Variant SKU. Resolves to one variant; a multi-match is refused with the candidates listed (use --variant-id, or --all)."},
		{Name: "product-id", Type: common.FlagString, Description: "Product ID. Resolves to that product's single variant; a multi-variant product is refused with the candidates listed."},
		{Name: "all", Type: common.FlagBool, Description: "With --sku only: update every variant matching the SKU."},
		{Name: "price", Type: common.FlagString, Required: true, Description: "New price (required, e.g. '24.99'; '0'/'0.00' clears it)."},
		{Name: "compare-price", Type: common.FlagString, Description: "New compare-at price."},
	},
	// Parses its own target, not parseVariantTarget: --variant-id with --sku is a
	// legitimate cross-check here, so exactly-one would delete it. Shared: the resolvers.
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		variantID := strings.TrimSpace(in.Flags.GetString("variant-id"))
		sku := strings.TrimSpace(in.Flags.GetString("sku"))
		productID := strings.TrimSpace(in.Flags.GetString("product-id"))
		all := in.Flags.GetBool("all")

		if variantID == "" && sku == "" && productID == "" {
			return common.ExecResult{}, output.ErrValidation("one of --variant-id, --sku or --product-id is required")
		}
		if productID != "" && (variantID != "" || sku != "") {
			return common.ExecResult{}, output.ErrValidation("--product-id cannot be combined with --variant-id or --sku").
				WithHint("--product-id resolves the product's only variant; pass --variant-id or --sku directly when you already know the target")
		}
		if all && variantID != "" {
			return common.ExecResult{}, output.ErrValidation("--all applies to --sku only; it cannot be combined with --variant-id")
		}
		if all && productID != "" {
			return common.ExecResult{}, output.ErrValidation("--all applies to --sku only; it cannot be combined with --product-id")
		}

		variantBody, err := buildVariantBody(in)
		if err != nil {
			return common.ExecResult{}, err
		}
		body := map[string]any{"variant": variantBody}

		switch {
		// Both given: verify the variant's SKU matches before updating by ID.
		case variantID != "" && sku != "":
			getPlan := PlanGetVariant(variantID)
			updatePlan := PlanUpdateVariant(variantID, body)
			if in.DryRun {
				return common.ExecResult{Plans: []common.PlannedRequest{getPlan, updatePlan}}, nil
			}
			getResp, err := common.Send(ctx, in.Client, getPlan)
			if err != nil {
				return common.ExecResult{}, translateVariantNotFound(err)
			}
			if actual := variantSKU(getResp); actual != sku {
				return common.ExecResult{}, output.ErrValidation("variant %s has SKU %q, which does not match --sku %q", variantID, actual, sku)
			}
			return sendUpdate(ctx, in, updatePlan)

		// Variant ID only: update that one.
		case variantID != "":
			res, err := single(ctx, in, PlanUpdateVariant(variantID, body))
			if err != nil {
				return common.ExecResult{}, translateVariantNotFound(err)
			}
			return res, nil

		// Product ID: resolve the product's only variant; refuse a multi-variant product.
		case productID != "":
			listPlan := PlanListVariantsForProduct(productID)
			if in.DryRun {
				return common.ExecResult{Plans: []common.PlannedRequest{listPlan, PlanUpdateVariant(stepRef(0), body)}}, nil
			}
			listResp, err := common.Send(ctx, in.Client, listPlan)
			if err != nil {
				return common.ExecResult{}, err
			}
			id, err := resolveOnlyVariant(listResp, productID)
			if err != nil {
				return common.ExecResult{}, err
			}
			return sendUpdate(ctx, in, PlanUpdateVariant(id, body))

		// SKU + --all: batch-update every variant with this SKU.
		case all:
			batch := map[string]any{"variant": variantBody, "refuse_multi_result": false}
			return single(ctx, in, PlanUpdateVariantBySKU(sku, batch))

		// SKU only: resolve to a single variant; refuse a multi-match.
		default:
			listPlan := PlanListVariantsBySKU(sku)
			if in.DryRun {
				return common.ExecResult{Plans: []common.PlannedRequest{listPlan, PlanUpdateVariant(stepRef(0), body)}}, nil
			}
			listResp, err := common.Send(ctx, in.Client, listPlan)
			if err != nil {
				return common.ExecResult{}, err
			}
			id, err := resolveSingleVariant(listResp, sku, true)
			if err != nil {
				return common.ExecResult{}, err
			}
			return sendUpdate(ctx, in, PlanUpdateVariant(id, body))
		}
	},
}

// single dry-runs or sends a one-shot plan.
func single(ctx context.Context, in common.ExecInput, plan common.PlannedRequest) (common.ExecResult, error) {
	if in.DryRun {
		return common.ExecResult{Plans: []common.PlannedRequest{plan}}, nil
	}
	return sendUpdate(ctx, in, plan)
}

func sendUpdate(ctx context.Context, in common.ExecInput, plan common.PlannedRequest) (common.ExecResult, error) {
	resp, err := common.Send(ctx, in.Client, plan)
	if err != nil {
		return common.ExecResult{}, err
	}
	return common.ExecResult{Body: resp}, nil
}

// buildVariantBody parses --price (required, >= 0) and --compare-price into the
// variant payload.
func buildVariantBody(in common.ExecInput) (map[string]any, error) {
	price, err := parsePrice("--price", in.Flags.GetString("price"))
	if err != nil {
		return nil, err
	}
	out := map[string]any{"price": price}
	if cp := in.Flags.GetString("compare-price"); cp != "" {
		cpf, err := strconv.ParseFloat(cp, 64)
		if err != nil {
			return nil, output.ErrValidation("--compare-price must be a number, got %q", cp)
		}
		out["compare_at_price"] = cpf
	}
	return out, nil
}

// variantSKU reads variant.sku from a `variants get` response ({"variant":{...}}).
func variantSKU(resp map[string]any) string {
	m, ok := resp["variant"].(map[string]any)
	if !ok {
		return ""
	}
	s, _ := m["sku"].(string)
	return s
}

// Identifier resolution lives in resolve.go — +stock shares it.
