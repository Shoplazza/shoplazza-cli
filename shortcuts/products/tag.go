package products

import (
	"context"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/internal/output"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
)

// tagShortcut edits a product's tags without clobbering the rest.
//
// PUT /products/{id} replaces the whole tags array, so adding one tag with the
// generated command wipes the others. --add/--remove read the current tags,
// merge, and write back; --set is the explicit full-replace escape hatch.
var tagShortcut = common.Shortcut{
	Service: "products",
	Command: "+tag",
	Use:     "+tag --id <product-id> (--add a,b | --remove c,d | --set x,y,z)",
	Short:   "Add or remove product tags without clobbering the existing ones",
	Flags: []common.Flag{
		common.IDFlag("Product ID (required)."),
		{Name: "add", Type: common.FlagStringSlice, Description: "Tags to add (comma-separated); existing tags are kept."},
		{Name: "remove", Type: common.FlagStringSlice, Description: "Tags to remove (comma-separated); missing tags are ignored."},
		{Name: "set", Type: common.FlagStringSlice, Description: "Replace all tags with exactly this list (mutually exclusive with --add/--remove)."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		id := strings.TrimSpace(in.Flags.GetString("id"))
		gotSet := in.Flags.Changed("set")
		gotAdd := in.Flags.Changed("add")
		gotRemove := in.Flags.Changed("remove")

		if gotSet && (gotAdd || gotRemove) {
			return common.ExecResult{}, output.ErrValidation("--set replaces all tags; it cannot be combined with --add or --remove")
		}
		if !gotSet && !gotAdd && !gotRemove {
			return common.ExecResult{}, output.ErrValidation("one of --add, --remove, or --set is required")
		}

		// --set: full replace, so no read-merge is needed.
		if gotSet {
			tags := normalizeTags(in.Flags.GetStringSlice("set"))
			return single(ctx, in, PlanUpdate(id, tagsBody(tags)))
		}

		// --add/--remove: read current tags, merge, write back.
		getPlan := PlanGet(id)
		if in.DryRun {
			preview := PlanUpdate(id, tagsBody([]string{"<current tags ∪ --add ∖ --remove>"}))
			return common.ExecResult{Plans: []common.PlannedRequest{getPlan, preview}}, nil
		}
		getResp, err := common.Send(ctx, in.Client, getPlan)
		if err != nil {
			return common.ExecResult{}, err
		}
		current := productTags(getResp)
		merged := mergeTags(current, in.Flags.GetStringSlice("add"), in.Flags.GetStringSlice("remove"))
		if equalTags(current, merged) {
			// Nothing changed: skip the pointless write, echo the product as-is.
			return common.ExecResult{Body: getResp}, nil
		}
		return sendUpdate(ctx, in, PlanUpdate(id, tagsBody(merged)))
	},
}

// tagsBody wraps a tag list in the {"product":{"tags":[...]}} update payload.
func tagsBody(tags []string) map[string]any {
	return map[string]any{"product": map[string]any{"tags": tags}}
}

// mergeTags applies remove (against existing) then add, returning a trimmed,
// deduplicated, order-preserving list. A tag in both --add and --remove ends up
// present (add wins), since remove only filters the existing set.
func mergeTags(existing, add, remove []string) []string {
	removeSet := map[string]bool{}
	for _, t := range remove {
		if t = strings.TrimSpace(t); t != "" {
			removeSet[t] = true
		}
	}
	out := []string{}
	seen := map[string]bool{}
	emit := func(tags []string, skipRemoved bool) {
		for _, t := range tags {
			t = strings.TrimSpace(t)
			if t == "" || seen[t] || (skipRemoved && removeSet[t]) {
				continue
			}
			seen[t] = true
			out = append(out, t)
		}
	}
	emit(existing, true)
	emit(add, false)
	return out
}

// normalizeTags trims, drops empties, and deduplicates while preserving order.
func normalizeTags(in []string) []string {
	return mergeTags(in, nil, nil)
}

// productTags reads product.tags from a `products get` response. JSON decoding
// yields []any of strings, so it handles both []any and []string.
func productTags(resp map[string]any) []string {
	prod, ok := resp["product"].(map[string]any)
	if !ok {
		return nil
	}
	switch raw := prod["tags"].(type) {
	case []string:
		return raw
	case []any:
		out := make([]string, 0, len(raw))
		for _, v := range raw {
			if s, ok := v.(string); ok {
				out = append(out, s)
			}
		}
		return out
	default:
		return nil
	}
}

// equalTags reports whether a and b hold the same tags in the same order.
func equalTags(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}
