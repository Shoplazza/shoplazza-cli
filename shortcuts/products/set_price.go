package products

import (
	"context"
	"fmt"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

var setPriceShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-price",
	Use:     "+set-price (--variant-id <id> | --sku <sku> [--all]) --price <n> [--compare-price <n>]",
	Short:   "Set a variant's price by variant ID or SKU",
	Flags: []common.Flag{
		{Name: "variant-id", Type: common.FlagString, Description: "Variant ID — the unique, exact target."},
		{Name: "sku", Type: common.FlagString, Description: "Variant SKU. Resolves to one variant; a multi-match is refused with the candidates listed (use --variant-id, or --all)."},
		{Name: "all", Type: common.FlagBool, Description: "With --sku only: update every variant matching the SKU."},
		{Name: "price", Type: common.FlagString, Required: true, Description: "New price (required, e.g. '24.99'; '0'/'0.00' clears it)."},
		{Name: "compare-price", Type: common.FlagString, Description: "New compare-at price."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		variantID := strings.TrimSpace(in.Flags.GetString("variant-id"))
		sku := strings.TrimSpace(in.Flags.GetString("sku"))
		all := in.Flags.GetBool("all")

		if variantID == "" && sku == "" {
			return common.ExecResult{}, output.ErrValidation("one of --variant-id or --sku is required")
		}
		if all && variantID != "" {
			return common.ExecResult{}, output.ErrValidation("--all applies to --sku only; it cannot be combined with --variant-id")
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
				return common.ExecResult{}, err
			}
			if actual := variantSKU(getResp); actual != sku {
				return common.ExecResult{}, output.ErrValidation("variant %s has SKU %q, which does not match --sku %q", variantID, actual, sku)
			}
			return sendUpdate(ctx, in, updatePlan)

		// Variant ID only: update that one.
		case variantID != "":
			return single(ctx, in, PlanUpdateVariant(variantID, body))

		// SKU + --all: batch-update every variant with this SKU.
		case all:
			batch := map[string]any{"variant": variantBody, "refuse_multi_result": false}
			return single(ctx, in, PlanUpdateVariantBySKU(sku, batch))

		// SKU only: resolve to a single variant; refuse a multi-match.
		default:
			listPlan := PlanListVariantsBySKU(sku)
			if in.DryRun {
				return common.ExecResult{Plans: []common.PlannedRequest{listPlan, PlanUpdateVariant("<resolved-from-step-0>", body)}}, nil
			}
			listResp, err := common.Send(ctx, in.Client, listPlan)
			if err != nil {
				return common.ExecResult{}, err
			}
			id, err := resolveSingleVariant(listResp, sku)
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
	priceStr := in.Flags.GetString("price")
	price, err := strconv.ParseFloat(priceStr, 64)
	if err != nil {
		return nil, output.ErrValidation("--price must be a number, got %q", priceStr)
	}
	if price < 0 {
		return nil, output.ErrValidation("--price must be >= 0, got %v", price)
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

// resolveSingleVariant returns the variant ID when exactly one variant in resp
// matches sku. Zero matches or more than one is an error (the latter lists the
// candidates so the caller can re-run with --variant-id or --all).
func resolveSingleVariant(resp map[string]any, sku string) (string, error) {
	matches := variantsMatchingSKU(resp, sku)
	switch len(matches) {
	case 0:
		return "", output.ErrValidation("no variant found with SKU %q", sku)
	case 1:
		id, _ := matches[0]["id"].(string)
		if id == "" {
			return "", output.ErrInternal("matched variant has no id")
		}
		return id, nil
	default:
		ids := make([]string, 0, len(matches))
		for _, m := range matches {
			if id, _ := m["id"].(string); id != "" {
				ids = append(ids, id)
			}
		}
		return "", output.ErrValidation("SKU %q matches %d variants", sku, len(matches)).
			WithHint(fmt.Sprintf("use --variant-id to target one of [%s], or --all to update them all", strings.Join(ids, ", ")))
	}
}

func variantsMatchingSKU(resp map[string]any, sku string) []map[string]any {
	raw, ok := resp["variants"].([]any)
	if !ok {
		return nil
	}
	var out []map[string]any
	for _, v := range raw {
		m, ok := v.(map[string]any)
		if !ok {
			continue
		}
		if s, _ := m["sku"].(string); s == sku {
			out = append(out, m)
		}
	}
	return out
}
