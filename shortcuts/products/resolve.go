package products

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/client"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// Product id → variant id resolution, shared by +set-price and +stock.

// variantTarget is a variant-level shortcut's target: exactly one selector set.
type variantTarget struct {
	VariantID string
	SKU       string
	ProductID string
}

// parseVariantTarget reads the three target flags and enforces exactly one.
func parseVariantTarget(in common.ExecInput) (variantTarget, error) {
	t := variantTarget{
		VariantID: strings.TrimSpace(in.Flags.GetString("variant-id")),
		SKU:       strings.TrimSpace(in.Flags.GetString("sku")),
		ProductID: strings.TrimSpace(in.Flags.GetString("product-id")),
	}
	n := 0
	for _, v := range []string{t.VariantID, t.SKU, t.ProductID} {
		if v != "" {
			n++
		}
	}
	switch n {
	case 1:
		return t, nil
	case 0:
		return variantTarget{}, output.ErrValidation("one of --variant-id, --sku or --product-id is required")
	default:
		return variantTarget{}, output.ErrValidation("--variant-id, --sku and --product-id are mutually exclusive; pass exactly one")
	}
}

// NeedsResolve reports whether a lookup is required to reach a variant id.
func (t variantTarget) NeedsResolve() bool { return t.VariantID == "" }

// ResolvePlan is the lookup that turns the selector into a variant list.
func (t variantTarget) ResolvePlan() common.PlannedRequest {
	if t.SKU != "" {
		return PlanListVariantsBySKU(t.SKU)
	}
	return PlanListVariantsForProduct(t.ProductID)
}

// Resolve runs the lookup and narrows it to one variant id.
func (t variantTarget) Resolve(ctx context.Context, c *client.Client) (string, error) {
	resp, err := common.Send(ctx, c, t.ResolvePlan())
	if err != nil {
		return "", err
	}
	if t.SKU != "" {
		return resolveSingleVariant(resp, t.SKU, false)
	}
	return resolveOnlyVariant(resp, t.ProductID)
}

// resolveSingleVariant returns the variant ID when exactly one variant matches sku;
// a multi-match is refused with the candidates listed. allowAll says whether the
// calling shortcut has --all (only +set-price does) — never name a flag it lacks.
func resolveSingleVariant(resp map[string]any, sku string, allowAll bool) (string, error) {
	matches := variantsMatchingSKU(resp, sku)
	switch len(matches) {
	case 0:
		return "", output.ErrValidation("no variant found with SKU %q", sku)
	case 1:
		id := asString(matches[0]["id"])
		if id == "" {
			return "", output.ErrInternal("matched variant has no id")
		}
		return id, nil
	default:
		hint := fmt.Sprintf("use --variant-id to target one of [%s]",
			candidateList(matches, fmt.Sprintf(`products variants list-by-sku --params '{"sku":"%s"}'`, sku)))
		if allowAll {
			hint += ", or --all to update them all"
		}
		return "", output.ErrValidation("SKU %q matches %d variants", sku, len(matches)).WithHint(hint)
	}
}

// resolveOnlyVariant returns the sole variant ID of a product-scoped variant list.
// A multi-variant product is refused: guessing variants[0] writes to the wrong
// sellable unit.
func resolveOnlyVariant(resp map[string]any, productID string) (string, error) {
	rows := variantRows(resp)
	switch len(rows) {
	case 0:
		return "", output.ErrValidation("product %s has no variants", productID)
	case 1:
		id := asString(rows[0]["id"])
		if id == "" {
			return "", output.ErrInternal("resolved variant has no id")
		}
		return id, nil
	default:
		return "", output.ErrValidation(
			"product %s has %d variants — a product id cannot identify one", productID, len(rows)).
			WithHint(fmt.Sprintf("ask which one, then re-run with --variant-id (or --sku): [%s]",
				candidateList(rows, fmt.Sprintf(`products variants list --params '{"product_id":"%s"}'`, productID))))
	}
}

// variantRows extracts the variant objects from any {"variants":[…]} response.
func variantRows(resp map[string]any) []map[string]any {
	raw, ok := resp["variants"].([]any)
	if !ok {
		return nil
	}
	out := make([]map[string]any, 0, len(raw))
	for _, v := range raw {
		if m, ok := v.(map[string]any); ok {
			out = append(out, m)
		}
	}
	return out
}

func variantsMatchingSKU(resp map[string]any, sku string) []map[string]any {
	var out []map[string]any
	for _, m := range variantRows(resp) {
		if s, _ := m["sku"].(string); s == sku {
			out = append(out, m)
		}
	}
	return out
}

// maxCandidates caps a refusal hint; a 45-variant product would otherwise dump
// every id onto one line.
const maxCandidates = 10

// candidateList renders up to maxCandidates labels, always stating what it dropped.
func candidateList(rows []map[string]any, listCmd string) string {
	labels := variantLabels(rows)
	if len(labels) <= maxCandidates {
		return strings.Join(labels, ", ")
	}
	return fmt.Sprintf("%s, … and %d more — list them all with: %s",
		strings.Join(labels[:maxCandidates], ", "), len(labels)-maxCandidates, listCmd)
}

// variantLabels renders candidates as "id (name)" so the caller can pick one
// without a second lookup; bare id when the row has no readable name.
func variantLabels(rows []map[string]any) []string {
	out := make([]string, 0, len(rows))
	for _, m := range rows {
		id := asString(m["id"])
		if id == "" {
			continue
		}
		if name := variantLabel(m); name != "" {
			out = append(out, fmt.Sprintf("%s (%s)", id, name))
			continue
		}
		out = append(out, id)
	}
	return out
}

func variantLabel(m map[string]any) string {
	if t, _ := m["title"].(string); strings.TrimSpace(t) != "" {
		return strings.TrimSpace(t)
	}
	var opts []string
	for _, k := range []string{"option1", "option2", "option3"} {
		if v, _ := m[k].(string); strings.TrimSpace(v) != "" {
			opts = append(opts, strings.TrimSpace(v))
		}
	}
	return strings.Join(opts, " / ")
}

// stepRef renders the dry-run placeholder for the value plan index i produces.
// Callers pass base+n, not a literal, so a prepended step cannot leave it stale.
func stepRef(i int) string { return fmt.Sprintf("<resolved-from-step-%d>", i) }

// variantIDHint recovers from a product id passed to --variant-id.
const variantIDHint = "--variant-id expects a VARIANT id. If you passed a PRODUCT id, use --product-id instead, " +
	`or list the variants: products variants list --params '{"product_id":"<id>"}'`

// translateVariantNotFound turns the 404 a product id earns on a variant-scoped
// path into a self-correcting error.
func translateVariantNotFound(err error) error {
	var httpErr *client.HTTPError
	if !errors.As(err, &httpErr) || httpErr.StatusCode != 404 {
		return err
	}
	return output.ErrAPI(httpErr.StatusCode, httpErr.Body, "").WithHint(variantIDHint)
}

// hintIfDirectVariantID adds the recovery hint to the 404 and the empty inventory
// lookup a product id earns. No-op unless the user typed --variant-id themselves:
// an id we resolved is ours, so those failures mean something else.
func (t variantTarget) hintIfDirectVariantID(err error) error {
	if err == nil || t.VariantID == "" {
		return err
	}
	if translated := translateVariantNotFound(err); translated != err {
		return translated
	}
	var exitErr *output.ExitError
	if errors.As(err, &exitErr) && exitErr.Detail != nil &&
		exitErr.Detail.Hint == "" && strings.HasPrefix(exitErr.Detail.Message, noInventoryItemMsg) {
		return exitErr.WithHint(variantIDHint)
	}
	return err
}
