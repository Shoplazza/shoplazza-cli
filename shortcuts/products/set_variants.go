package products

import (
	"context"
	"fmt"
	"reflect"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// setVariantsShortcut edits an existing product's option matrix and generates
// the variants from the cartesian product. `--option` carries the payload
// (dimension name + values); `--action` is the verb; unstated dimensions and
// values are preserved.
//
// While dimension names are unchanged, existing variants are matched to new
// combos by option values and keep their id (the API then preserves fields
// absent from the body: sku, inventory, image). When the dimension set
// changes, every variant is rebuilt and each deleted one is listed in the
// output. The output is a bounded summary, never the full product.
var setVariantsShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-variants",
	Use:     `+set-variants --id <product-id> --action <add|remove|update> --option "Color:Red,Blue" [--option ...] [--price <n>]`,
	Short:   "Edit a product's option matrix (add/remove/update spec dimensions and values); variants are generated",
	Long: `Edit an existing product's specification matrix; the CLI merges the change into
the current dimensions, expands the cartesian product into the full variant
list, and submits it in one request.

--option names the dimensions/values to operate on and --action says what to
do (exactly one action per call, max 3 dimensions):
  add     append values to a dimension, or add a whole new dimension
  remove  drop values from a dimension; a bare name ("Size") drops the dimension
  update  replace one dimension's value list with exactly the given values
There is no full-declaration action: unstated dimensions and values are always
preserved.

To create a multi-spec product, create it first (products +create), then add
the dimensions here with --action add.

Identity rules: while the dimension NAMES are unchanged, existing variants
whose values match a new combination keep their id, and the API preserves any
field not present in the body (sku, inventory, image). When the dimension set
changes (added/removed/renamed), every variant is rebuilt and all old variants
are deleted — each is listed in the output with its id/sku/inventory.
Deleted variants do not break past orders (line items are snapshotted and the
order can still be paid/fulfilled); only variant-keyed references go stale.

--price applies to newly created variants only; matched variants keep their
current price. Renaming a value or dimension is not detected as a rename: the
old one is deleted and the new one created.

The output is a bounded summary ({created, inherited, deleted} + deleted
detail), never the full product body. Preview requests with --dry-run.`,
	Flags: []common.Flag{
		{Name: "id", Type: common.FlagString, Required: true, Description: "Product ID (required)."},
		{Name: "action", Type: common.FlagString, Required: true,
			Description: "What to do with the --option payloads: add, remove, or update.",
			Completions: []string{"add", "remove", "update"}},
		{Name: "option", Type: common.FlagStringArray, Required: true,
			Description: `Dimension payload as "Name:v1,v2,..." (repeat per dimension; values comma-separated, order = storefront order). Bare "Name" only with --action remove.`},
		{Name: "price", Type: common.FlagString, Description: "Price for newly created variants (required whenever new combinations appear). Matched variants keep their current price."},
		{Name: "sku-template", Type: common.FlagString, Description: `SKU pattern with {Option Name} placeholders, e.g. "TS-{Color}-{Size}". Applied to EVERY variant (overwrites inherited skus).`},
		{Name: "stock", Type: common.FlagInt, Description: "Inventory quantity set on EVERY variant (absolute set at the default location; overwrites inherited stock). Omit to leave stock untouched."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		id := strings.TrimSpace(in.Flags.GetString("id"))
		if id == "" {
			return common.ExecResult{}, output.ErrValidation("--id is required")
		}
		action := strings.TrimSpace(in.Flags.GetString("action"))
		switch action {
		case "add", "remove", "update":
		default:
			return common.ExecResult{}, output.ErrValidation("unknown --action %q: use add, remove, or update", action)
		}

		price, hasPrice, err := parseOptionalPrice(in.Flags.GetString("price"))
		if err != nil {
			return common.ExecResult{}, err
		}
		specs, err := parseOptionSpecs(in.Flags.GetStringArray("option"), action == "remove")
		if err != nil {
			return common.ExecResult{}, err
		}
		return execUpdateMatrix(ctx, in, id, action, specs, price, hasPrice,
			in.Flags.GetString("sku-template"), in.Flags.Changed("stock"), in.Flags.GetInt("stock"))
	},
}

// ── option parsing ────────────────────────────────────────────────────────────

// optionDim is one parsed --option payload. Values==nil means a bare "Name"
// (whole-dimension reference, only legal under --action remove).
type optionDim struct {
	Name   string
	Values []string
}

// parseOptionSpecs parses repeated `--option "Name:v1,v2"` payloads.
func parseOptionSpecs(specs []string, allowBareName bool) ([]optionDim, error) {
	if len(specs) == 0 {
		return nil, output.ErrValidation(`at least one --option "Name:v1,v2,..." is required`)
	}
	dims := make([]optionDim, 0, len(specs))
	seenNames := map[string]bool{}
	for _, spec := range specs {
		name, rawValues, hasColon := strings.Cut(spec, ":")
		name = strings.TrimSpace(name)
		if name == "" {
			return nil, output.ErrValidation(`--option must be "Name:v1,v2,...", got %q`, spec)
		}
		if key := normKey(name); seenNames[key] {
			return nil, output.ErrValidation("duplicate option name %q", name)
		} else {
			seenNames[key] = true
		}
		var values []string
		seenValues := map[string]bool{}
		for v := range strings.SplitSeq(rawValues, ",") {
			v = strings.TrimSpace(v)
			if v == "" {
				continue
			}
			if key := normKey(v); seenValues[key] {
				return nil, output.ErrValidation("option %q has duplicate value %q", name, v)
			} else {
				seenValues[key] = true
			}
			values = append(values, v)
		}
		if len(values) == 0 {
			if !hasColon && allowBareName {
				dims = append(dims, optionDim{Name: name})
				continue
			}
			if !hasColon {
				return nil, output.ErrValidation(`--option must be "Name:v1,v2,..." (a bare name is only valid with --action remove), got %q`, spec)
			}
			return nil, output.ErrValidation("option %q has no values", name)
		}
		dims = append(dims, optionDim{Name: name, Values: values})
	}
	if len(dims) > 3 && !allowBareName {
		return nil, output.ErrValidation("at most 3 option dimensions are supported (the platform caps variants at option1/2/3), got %d", len(dims))
	}
	return dims, nil
}

// cartesian expands dimensions into ordered value tuples (first dimension
// varies slowest).
func cartesian(dims []optionDim) [][]string {
	combos := [][]string{{}}
	for _, d := range dims {
		next := make([][]string, 0, len(combos)*len(d.Values))
		for _, c := range combos {
			for _, v := range d.Values {
				row := make([]string, len(c), len(c)+1)
				copy(row, c)
				next = append(next, append(row, v))
			}
		}
		combos = next
	}
	return combos
}

// normKey normalizes an option name or value for matching only — the original
// spelling is always what gets written.
func normKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

func comboKey(values []string) string {
	keys := make([]string, len(values))
	for i, v := range values {
		keys[i] = normKey(v)
	}
	return strings.Join(keys, "\x00")
}

// parseOptionalPrice parses --price when present. hasPrice=false means the
// flag was omitted (allowed until a new variant actually needs it).
func parseOptionalPrice(s string) (float64, bool, error) {
	if strings.TrimSpace(s) == "" {
		return 0, false, nil
	}
	p, err := strconv.ParseFloat(s, 64)
	if err != nil {
		return 0, false, output.ErrValidation("--price must be a number, got %q", s)
	}
	if p < 0 {
		return 0, false, output.ErrValidation("--price must be >= 0, got %v", p)
	}
	return p, true, nil
}

// validateSKUTemplate rejects placeholders that name no dimension of the final matrix.
func validateSKUTemplate(tpl string, dims []optionDim) error {
	if tpl == "" {
		return nil
	}
	rest := tpl
	for {
		_, after, ok := strings.Cut(rest, "{")
		if !ok {
			return nil
		}
		ph, tail, ok := strings.Cut(after, "}")
		if !ok {
			return output.ErrValidation("--sku-template has an unclosed placeholder: %q", tpl)
		}
		found := false
		for _, d := range dims {
			if normKey(d.Name) == normKey(ph) {
				found = true
				break
			}
		}
		if !found {
			return output.ErrValidation("--sku-template placeholder {%s} matches no option dimension", ph)
		}
		rest = tail
	}
}

// renderSKU fills {Option Name} placeholders with the combo's values.
func renderSKU(tpl string, dims []optionDim, combo []string) string {
	out := tpl
	for i, d := range dims {
		out = strings.ReplaceAll(out, "{"+d.Name+"}", combo[i])
	}
	return out
}

// optionsBody renders dims as the API's options array.
func optionsBody(dims []optionDim) []map[string]any {
	out := make([]map[string]any, len(dims))
	for i, d := range dims {
		out[i] = map[string]any{"name": d.Name, "values": d.Values}
	}
	return out
}

// setComboOptions writes a combo's values into option1/2/3.
func setComboOptions(variant map[string]any, combo []string) {
	for i, v := range combo {
		variant[fmt.Sprintf("option%d", i+1)] = v
	}
}

func execUpdateMatrix(ctx context.Context, in common.ExecInput, id, action string, specs []optionDim,
	price float64, hasPrice bool, skuTemplate string, hasStock bool, stock int) (common.ExecResult, error) {

	getPlan := PlanGet(id)
	if in.DryRun {
		preview := PlanUpdate(id, map[string]any{"product": map[string]any{
			"options": fmt.Sprintf("<current dimensions after --action %s>", action),
			"variants": "<cartesian product of the merged matrix; while dimension names are unchanged, " +
				"existing variant ids are matched in by value and their sku/inventory preserved; otherwise a full rebuild>",
			"has_only_default_variant": false,
		}})
		return common.ExecResult{Plans: []common.PlannedRequest{getPlan, preview}}, nil
	}

	getResp, err := common.Send(ctx, in.Client, getPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	currentDims, oldVariants := readProductMatrix(getResp)

	dims, err := applyMatrixAction(currentDims, action, specs)
	if err != nil {
		return common.ExecResult{}, err
	}
	if err := validateSKUTemplate(skuTemplate, dims); err != nil {
		return common.ExecResult{}, err
	}

	// No effective change and no per-variant override: skip the write.
	if reflect.DeepEqual(dims, currentDims) && skuTemplate == "" && !hasStock {
		summary := matrixSummary(id, dims, len(oldVariants))
		summary["action"] = action
		summary["dimension_change"] = false
		summary["created"] = 0
		summary["inherited"] = len(oldVariants)
		summary["deleted"] = 0
		summary["no_change"] = true
		return common.ExecResult{Body: summary}, nil
	}

	// Dimension sets are compared by option NAME (normalized): same names in
	// any order → variants are matchable; otherwise a full rebuild.
	sameDims := len(currentDims) == len(dims)
	oldPosByName := map[string]int{} // normalized name → 1-based old option position
	for i, cd := range currentDims {
		oldPosByName[normKey(cd.Name)] = i + 1
		if sameDims {
			found := false
			for _, d := range dims {
				if normKey(d.Name) == normKey(cd.Name) {
					found = true
					break
				}
			}
			sameDims = found
		}
	}

	// Index old variants by their value tuple projected onto the NEW dimension
	// order. Only meaningful when the dimension sets match.
	oldByKey := map[string]oldVariant{}
	if sameDims {
		for _, ov := range oldVariants {
			values := make([]string, len(dims))
			for i, d := range dims {
				values[i] = ov.optionValue(oldPosByName[normKey(d.Name)])
			}
			key := comboKey(values)
			if _, dup := oldByKey[key]; !dup {
				oldByKey[key] = ov
			}
		}
	}

	combos := cartesian(dims)
	if !hasPrice {
		for _, combo := range combos {
			if _, ok := oldByKey[comboKey(combo)]; !ok {
				return common.ExecResult{}, output.ErrValidation(
					"new variant(s) would be created (e.g. %q) — pass --price to give them one",
					strings.Join(combo, "/"))
			}
		}
	}

	variants := make([]any, len(combos))
	matched := map[string]bool{} // old variant ids that found a combo
	inherited := 0
	created := 0
	for i, combo := range combos {
		v := map[string]any{}
		setComboOptions(v, combo)
		if ov, ok := oldByKey[comboKey(combo)]; ok && !matched[ov.ID] {
			matched[ov.ID] = true
			v["id"] = ov.ID
			inherited++
		} else {
			created++
			v["price"] = price
		}
		if skuTemplate != "" {
			v["sku"] = renderSKU(skuTemplate, dims, combo)
		}
		if hasStock {
			v["inventory_quantity"] = stock
		}
		variants[i] = v
	}

	var deletedDetail []map[string]any
	for _, ov := range oldVariants {
		if !matched[ov.ID] {
			deletedDetail = append(deletedDetail, ov.detail())
		}
	}

	body := map[string]any{"product": map[string]any{
		"options":                  optionsBody(dims),
		"variants":                 variants,
		"has_only_default_variant": false,
	}}
	resp, err := common.Send(ctx, in.Client, PlanUpdate(id, body))
	if err != nil {
		return common.ExecResult{}, err
	}

	summary := matrixSummary(id, dims, respVariantCount(resp, len(combos)))
	summary["action"] = action
	summary["dimension_change"] = !sameDims
	summary["created"] = created
	summary["inherited"] = inherited
	summary["deleted"] = len(deletedDetail)
	addDetail(summary, "deleted_detail", deletedDetail)
	if skuTemplate != "" {
		summary["sku_template_applied"] = true
	}
	if hasStock {
		summary["stock_applied"] = stock
	}
	return common.ExecResult{Body: summary}, nil
}

// applyMatrixAction merges the --option payloads into the current dimensions.
//
//	add:    append values (normalized dedup); an unknown name adds a new dimension.
//	update: replace one dimension's value list; unknown name is an error.
//	remove: drop values; a bare name (or emptying a dimension) drops the whole
//	        dimension; missing names/values are no-ops.
func applyMatrixAction(current []optionDim, action string, specs []optionDim) ([]optionDim, error) {
	// Deep-copy so the caller's view of the current matrix stays intact.
	dims := make([]optionDim, len(current))
	for i, d := range current {
		dims[i] = optionDim{Name: d.Name, Values: append([]string{}, d.Values...)}
	}
	find := func(name string) int {
		for i, d := range dims {
			if normKey(d.Name) == normKey(name) {
				return i
			}
		}
		return -1
	}

	for _, spec := range specs {
		i := find(spec.Name)
		switch action {
		case "add":
			if i < 0 {
				dims = append(dims, optionDim{Name: spec.Name, Values: spec.Values})
				continue
			}
			have := map[string]bool{}
			for _, v := range dims[i].Values {
				have[normKey(v)] = true
			}
			for _, v := range spec.Values {
				if !have[normKey(v)] {
					dims[i].Values = append(dims[i].Values, v)
				}
			}
		case "update":
			if i < 0 {
				return nil, output.ErrValidation("option %q does not exist on this product — to add a new dimension use --action add", spec.Name)
			}
			dims[i].Values = spec.Values
		case "remove":
			if i < 0 {
				continue // already absent: converged
			}
			if spec.Values == nil {
				dims = append(dims[:i], dims[i+1:]...)
				continue
			}
			drop := map[string]bool{}
			for _, v := range spec.Values {
				drop[normKey(v)] = true
			}
			kept := dims[i].Values[:0]
			for _, v := range dims[i].Values {
				if !drop[normKey(v)] {
					kept = append(kept, v)
				}
			}
			if len(kept) == 0 {
				dims = append(dims[:i], dims[i+1:]...)
			} else {
				dims[i].Values = kept
			}
		}
	}

	if len(dims) == 0 {
		return nil, output.ErrValidation("this would remove every dimension and leave the product without variants — delete the product or keep at least one dimension")
	}
	if len(dims) > 3 {
		return nil, output.ErrValidation("at most 3 option dimensions are supported (the platform caps variants at option1/2/3), got %d", len(dims))
	}
	return dims, nil
}

// ── GET-response readers ──────────────────────────────────────────────────────

// oldVariant is the slice of an existing variant the mapping needs.
type oldVariant struct {
	ID        string
	Options   [3]string // option1..option3, "" when absent
	SKU       string
	Inventory *int
}

func (ov oldVariant) optionValue(pos int) string {
	if pos < 1 || pos > 3 {
		return ""
	}
	return ov.Options[pos-1]
}

// detail renders the old variant for the summary's deleted list.
func (ov oldVariant) detail() map[string]any {
	parts := []string{}
	for _, v := range ov.Options {
		if v != "" {
			parts = append(parts, v)
		}
	}
	d := map[string]any{"id": ov.ID, "options": strings.Join(parts, "/")}
	if ov.SKU != "" {
		d["sku"] = ov.SKU
	}
	if ov.Inventory != nil {
		d["inventory_quantity"] = *ov.Inventory
	}
	return d
}

// readProductMatrix extracts the option dimensions (ordered by position) and
// variants from a `products get` response.
func readProductMatrix(resp map[string]any) (dims []optionDim, variants []oldVariant) {
	prod, _ := resp["product"].(map[string]any)
	if prod == nil {
		return nil, nil
	}
	if rawOpts, ok := prod["options"].([]any); ok {
		type posDim struct {
			pos int
			dim optionDim
		}
		named := make([]posDim, 0, len(rawOpts))
		for i, ro := range rawOpts {
			m, _ := ro.(map[string]any)
			if m == nil {
				continue
			}
			name, _ := m["name"].(string)
			if name == "" {
				continue
			}
			pos := i + 1
			if p, ok := asInt(m["position"]); ok && p >= 1 {
				pos = p
			}
			var values []string
			if rawVals, ok := m["values"].([]any); ok {
				for _, rv := range rawVals {
					if s, ok := rv.(string); ok && strings.TrimSpace(s) != "" {
						values = append(values, s)
					}
				}
			}
			named = append(named, posDim{pos: pos, dim: optionDim{Name: name, Values: values}})
		}
		for i := 0; i < len(named); i++ { // insertion sort: options are ≤3
			for j := i; j > 0 && named[j].pos < named[j-1].pos; j-- {
				named[j], named[j-1] = named[j-1], named[j]
			}
		}
		for _, pn := range named {
			dims = append(dims, pn.dim)
		}
	}
	if rawVars, ok := prod["variants"].([]any); ok {
		for _, rv := range rawVars {
			m, _ := rv.(map[string]any)
			if m == nil {
				continue
			}
			ov := oldVariant{}
			ov.ID = asString(m["id"])
			for i := range 3 {
				if s, ok := m[fmt.Sprintf("option%d", i+1)].(string); ok {
					ov.Options[i] = s
				}
			}
			ov.SKU, _ = m["sku"].(string)
			if q, ok := asInt(m["inventory_quantity"]); ok {
				ov.Inventory = &q
			}
			if ov.ID != "" {
				variants = append(variants, ov)
			}
		}
	}
	// Derive a dimension's values from the variants when the options array omits them.
	for i := range dims {
		if len(dims[i].Values) > 0 {
			continue
		}
		seen := map[string]bool{}
		for _, ov := range variants {
			v := ov.optionValue(i + 1)
			if v != "" && !seen[normKey(v)] {
				seen[normKey(v)] = true
				dims[i].Values = append(dims[i].Values, v)
			}
		}
	}
	return dims, variants
}

// ── summary helpers ───────────────────────────────────────────────────────────

// detailCap bounds the deleted list so the summary stays small even for huge
// matrices; the truncated count is reported alongside.
const detailCap = 100

func matrixSummary(productID string, dims []optionDim, variantsTotal int) map[string]any {
	opts := make([]map[string]any, len(dims))
	for i, d := range dims {
		opts[i] = map[string]any{"name": d.Name, "values": len(d.Values)}
	}
	return map[string]any{
		"product_id":     productID,
		"options":        opts,
		"variants_total": variantsTotal,
	}
}

func addDetail(summary map[string]any, key string, detail []map[string]any) {
	if len(detail) == 0 {
		return
	}
	if len(detail) > detailCap {
		summary[key+"_truncated"] = len(detail) - detailCap
		detail = detail[:detailCap]
	}
	summary[key] = detail
}

// respVariantCount prefers the server's variant count; falls back to the
// planned count when the response shape is unexpected.
func respVariantCount(resp map[string]any, planned int) int {
	if prod, ok := resp["product"].(map[string]any); ok {
		if vars, ok := prod["variants"].([]any); ok {
			return len(vars)
		}
	}
	return planned
}

