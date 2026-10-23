package products

import (
	"context"
	"fmt"
	"slices"
	"strconv"
	"strings"
	"unicode"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// setVariantsShortcut edits a product's option matrix, keeping each variant's
// id wherever it maps onto the new matrix unambiguously. Any mix of edits goes
// in one call and one write.
var setVariantsShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-variants",
	Use:     `+set-variants --id <product-id> [--add|--remove|--set|--drop|--rename ...] [--new-price ...] [--set-price ...]`,
	Short:   "Edit a product's option matrix (add/remove/set/rename values, add/drop/rename dimensions, price variants) in one request",
	Example: `  # Drop lenses keeping the 6lenses variants, keep one style and rename it, one price for all
  shoplazza products +set-variants --id 12345 --drop "lenses=6lenses" --set "Style:Ocean" \
    --rename "Style:Ocean=2 lenses" --set-price 19.9 --dry-run

  # Add a Size dimension (existing variants become M) and a color with its own image
  shoplazza products +set-variants --id 12345 --add "Size:S,M,L" --existing "Size:M" \
    --add "Color:Red" --new-image "Color:Red=https://img.example.com/red.png" --new-price 24.99

  # Price per value
  shoplazza products +set-variants --id 12345 --set-price "Size:S=20,M=30,L=40"`,
	Long: `Edit an existing product's specification matrix in one request: the CLI reads
the product, applies every edit to it and writes the full variant list back.

Edits (repeatable, combinable). Dimensions and values are named as they are
on the product now:
  --remove "Dim:v1,v2"    delete these values and the variants using them
                          (exact spelling)
  --drop "Dim"            delete a dimension; variants keep their id
  --drop "Dim=value"      keep only the variants with that value, then delete
                          the dimension; required when variants would merge
  --set "Dim:v1,v2"       make the values exactly these, in this storefront
                          order: unlisted values go with their variants, new
                          ones get variants
  --add "Dim:v1,v2"       add values, one new variant per existing combination
                          of the other dimensions (deleted combinations stay
                          deleted); an unknown name adds a dimension
  --existing "Dim:value"  the value existing variants take in a dimension
                          added with several values (required then, unless
                          the product has no dimension yet: its one variant
                          takes the first value)
  --rename "Dim:old=new,..." / "OldDim=NewDim"   rename in place
They apply in that order whatever order they are written in; renames come
last, so every other edit uses the current names.

Existing variants keep their id, and with it sku, stock, image and every
other field. A sibling created by adding a dimension copies price, compare-at
and cost price, weight, and the image on products that require one (never
sku, barcode or stock). A value whose last variant goes is dropped from its
dimension; values that had no variant are left alone.

Values for variants, named as they are AFTER the edit:
  --new-price  P | "Dim:v=P,..."   price for new variants with nothing to copy
  --new-image  I | "Dim:v=I,..."   image (id or URL of one of the product's
                                   images) for new variants with none to copy;
                                   required on products that require one
  --new-stock  N                   stock for every new variant
  --set-price  P | "Dim:v=P,..."   price for EVERY variant (per value: the
                                   variants with that value)
  --set-stock  N                   stock for every variant
  --sku-template "TS-{Color}-{Size}"   sku for every variant
A per-value form names one dimension; a plain value may be passed alongside
it as the fallback for the other values.

Refused before anything is written: a name or value that does not exist, the
same dimension in two of --add/--remove/--set/--drop, a value both removed
and renamed, a dimension both dropped and renamed, a drop that would merge
variants, more than 3 dimensions or 300 variants, no dimension left, a new
variant without a price, or without an image on a product that requires one.

Deleted variants do not break past orders (line items are snapshotted). The
output is a bounded summary ({kept, cloned, created, deleted} + details, and
"changes" describing the edit in plain words), never the full product body.
--dry-run reads the product and prints the exact request plus the summary.`,
	DryRunReads: true,
	Flags: []common.Flag{
		{Name: "id", Type: common.FlagString, Required: true, Description: "Product ID (required)."},
		{Name: "add", Type: common.FlagStringArray, Description: `"Dim:v1,v2": add values; an unknown dimension name adds that dimension.`},
		{Name: "remove", Type: common.FlagStringArray, Description: `"Dim:v1,v2": delete values and the variants using them.`},
		{Name: "set", Type: common.FlagStringArray, Description: `"Dim:v1,v2": make the dimension's values exactly these (order = storefront order).`},
		{Name: "drop", Type: common.FlagStringArray, Description: `"Dim" or "Dim=value": delete a dimension, keeping only the variants with value when given.`},
		{Name: "rename", Type: common.FlagStringArray, Description: `"Dim:old=new,..." renames values; "OldDim=NewDim" renames a dimension.`},
		{Name: "existing", Type: common.FlagStringArray, Description: `"Dim:value": for a dimension added with several values, the value existing variants take.`},
		{Name: "new-price", Type: common.FlagStringArray, Description: `Price for new variants with nothing to copy: a number, or "Dim:v=price,..." per value.`},
		{Name: "new-image", Type: common.FlagStringArray, Description: `Image (id or URL of one of the product's images) for new variants with none to copy, or "Dim:v=image,..." per value.`},
		{Name: "new-stock", Type: common.FlagInt, Description: "Inventory quantity for every new variant."},
		{Name: "set-price", Type: common.FlagStringArray, Description: `Price for EVERY variant: a number, or "Dim:v=price,..." for the variants with those values.`},
		{Name: "set-stock", Type: common.FlagInt, Description: "Inventory quantity for EVERY variant (absolute, default location)."},
		{Name: "sku-template", Type: common.FlagString, Description: `SKU pattern with {Option Name} placeholders (names after the edit), e.g. "TS-{Color}-{Size}". Applied to EVERY variant.`},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		id := strings.TrimSpace(in.Flags.GetString("id"))
		if id == "" {
			return common.ExecResult{}, output.ErrValidation("--id is required")
		}
		plan, err := parseEditFlags(in)
		if err != nil {
			return common.ExecResult{}, err
		}
		return execEditPlan(ctx, in, id, plan)
	},
}

// matrixOverrides carries the per-variant flags.
type matrixOverrides struct {
	newPrice    perValue // prices new variants with nothing to copy
	setPrice    perValue // prices every variant it covers
	newImage    perValue // images new variants with none to copy
	skuTemplate string
	hasStock    bool // stock set on every variant
	stock       int
	hasNewStock bool // stock set on new variants
	newStock    int
	changes     []string
}

// ── option parsing ────────────────────────────────────────────────────────────

// optionDim is one dimension of a product's matrix.
type optionDim struct {
	Name   string
	Values []string
	// pos is the server-side option slot (1-based) for a dimension read from a
	// product; 0 for a dimension added by this call.
	pos int
	// idle holds values that no variant used when read; they are never pruned.
	idle map[string]bool
}

// slot returns the option1/2/3 slot this dimension's values live in on the
// server, falling back when the dimension carries no position.
func (d optionDim) slot(fallback int) int {
	if d.pos >= 1 {
		return d.pos
	}
	return fallback
}

// moveIdle carries idle marks across respellings (old → new).
func (d *optionDim) moveIdle(respell map[string]string) {
	for old, v := range respell {
		if old != v && d.idle[old] {
			delete(d.idle, old)
			d.idle[v] = true
		}
	}
}

// optionSpec is one parsed --add/--remove/--set/--rename payload.
type optionSpec struct {
	Name    string
	Values  []string
	Renames [][2]string // rename: old → new value
	NewName string      // rename: new dimension name
}

// parseValueSpec parses `Name:v1,v2`.
func parseValueSpec(spec string) (optionSpec, error) {
	name, rawValues, hasColon := strings.Cut(spec, ":")
	name = strings.TrimSpace(name)
	if name == "" || !hasColon {
		return optionSpec{}, output.ErrValidation(`takes "Dim:v1,v2,...", got %q`, spec)
	}
	var values []string
	for v := range strings.SplitSeq(rawValues, ",") {
		v = strings.TrimSpace(v)
		if v == "" {
			continue
		}
		if slices.Contains(values, v) {
			return optionSpec{}, output.ErrValidation("option %q has duplicate value %q", name, v)
		}
		values = append(values, v)
	}
	if len(values) == 0 {
		return optionSpec{}, output.ErrValidation("option %q has no values", name)
	}
	return optionSpec{Name: name, Values: values}, nil
}

// parseRenameSpec parses `Name:old=new,...` (values) or `Old=New` (a dimension).
func parseRenameSpec(spec string) (optionSpec, error) {
	name, rawPairs, hasColon := strings.Cut(spec, ":")
	if !hasColon {
		old, newName, ok := strings.Cut(spec, "=")
		old, newName = strings.TrimSpace(old), strings.TrimSpace(newName)
		if !ok || old == "" || newName == "" {
			return optionSpec{}, output.ErrValidation(`takes "Dim:old=new,..." or "OldDim=NewDim", got %q`, spec)
		}
		return optionSpec{Name: old, NewName: newName}, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return optionSpec{}, output.ErrValidation(`takes "Dim:old=new,..." or "OldDim=NewDim", got %q`, spec)
	}
	var pairs [][2]string
	for p := range strings.SplitSeq(rawPairs, ",") {
		if strings.TrimSpace(p) == "" {
			continue
		}
		old, newValue, ok := strings.Cut(p, "=")
		old, newValue = strings.TrimSpace(old), strings.TrimSpace(newValue)
		if !ok || old == "" || newValue == "" {
			return optionSpec{}, output.ErrValidation(`rename pair must be "old=new", got %q`, strings.TrimSpace(p))
		}
		for _, q := range pairs {
			if q[0] == old {
				return optionSpec{}, output.ErrValidation("option %q renames %q twice", name, old)
			}
		}
		pairs = append(pairs, [2]string{old, newValue})
	}
	if len(pairs) == 0 {
		return optionSpec{}, output.ErrValidation("option %q has no values", name)
	}
	return optionSpec{Name: name, Renames: pairs}, nil
}

// foldKey normalizes a name or value for the case-insensitive fallback only —
// the original spelling is always what gets written.
func foldKey(s string) string {
	return strings.ToLower(strings.TrimSpace(s))
}

// lookup finds s in have: an exact match first, else the only entry equal to it
// ignoring case. ambiguous reports several case-insensitive candidates.
func lookup(have []string, s string) (idx int, ambiguous bool) {
	if i := slices.Index(have, s); i >= 0 {
		return i, false
	}
	idx = -1
	for i, h := range have {
		if foldKey(h) == foldKey(s) {
			if idx >= 0 {
				return -1, true
			}
			idx = i
		}
	}
	return idx, false
}

func tupleKey(values []string) string {
	return strings.Join(values, "\x00")
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
			if foldKey(d.Name) == foldKey(ph) {
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
// Placeholders match dimension names by foldKey — the same rule
// validateSKUTemplate accepts them with.
func renderSKU(tpl string, dims []optionDim, combo []string) string {
	var b strings.Builder
	rest := tpl
	for {
		before, after, ok := strings.Cut(rest, "{")
		b.WriteString(before)
		if !ok {
			return b.String()
		}
		ph, tail, ok := strings.Cut(after, "}")
		if !ok { // unclosed; validateSKUTemplate rejects this earlier
			b.WriteString("{" + after)
			return b.String()
		}
		replaced := false
		for i, d := range dims {
			if foldKey(d.Name) == foldKey(ph) {
				b.WriteString(combo[i])
				replaced = true
				break
			}
		}
		if !replaced { // unknown; validateSKUTemplate rejects this earlier
			b.WriteString("{" + ph + "}")
		}
		rest = tail
	}
}

// optionsBody renders dims as the API's options array.
func optionsBody(dims []optionDim) []map[string]any {
	out := make([]map[string]any, len(dims))
	for i, d := range dims {
		out[i] = map[string]any{"name": d.Name, "values": d.Values}
	}
	return out
}

// ── matrix ────────────────────────────────────────────────────────────────────

// matrix is the working copy of a product's option matrix.
type matrix struct {
	dims []optionDim
	rows []*row
	// reshaped records that a dimension was added or removed.
	reshaped bool
	// renamed holds dimension names assigned by this call's renames.
	renamed map[string]bool
}

// row is one planned variant.
type row struct {
	values []string    // aligned with matrix.dims
	read   []string    // values as read, for an old variant's row
	src    *oldVariant // the old variant this row keeps; its id is sent
	clone  *oldVariant // the old variant a new sibling copies fields from
}

func newMatrix(dims []optionDim, olds []oldVariant) *matrix {
	m := &matrix{dims: make([]optionDim, len(dims))}
	for i, d := range dims {
		m.dims[i] = optionDim{Name: d.Name, Values: slices.Clone(d.Values), pos: d.pos}
	}
	for i := range olds {
		values := make([]string, len(dims))
		for d := range dims {
			values[d] = olds[i].optionValue(dims[d].slot(d + 1))
		}
		m.rows = append(m.rows, &row{values: values, read: slices.Clone(values), src: &olds[i]})
	}
	for d := range m.dims {
		used := map[string]bool{}
		for _, r := range m.rows {
			used[r.values[d]] = true
		}
		for _, v := range m.dims[d].Values {
			if !used[v] {
				if m.dims[d].idle == nil {
					m.dims[d].idle = map[string]bool{}
				}
				m.dims[d].idle[v] = true
			}
		}
	}
	return m
}

func (m *matrix) names() []string {
	names := make([]string, len(m.dims))
	for i, d := range m.dims {
		names[i] = d.Name
	}
	return names
}

// describe renders the dimensions for an error hint.
func (m *matrix) describe() string {
	parts := make([]string, len(m.dims))
	for i, d := range m.dims {
		parts[i] = d.Name + "[" + strings.Join(d.Values, ",") + "]"
	}
	if len(parts) == 0 {
		return "none"
	}
	return strings.Join(parts, ", ")
}

func (m *matrix) findDim(name string) (int, error) {
	i, ambiguous := lookup(m.names(), name)
	if ambiguous {
		return -1, output.ErrValidation("option name %q matches several dimensions ignoring case — use the exact spelling (existing: %s)", name, m.describe())
	}
	return i, nil
}

// findValue resolves v in dimension d; a miss is an error.
func (m *matrix) findValue(d int, v string) (int, error) {
	dim := m.dims[d]
	i, ambiguous := lookup(dim.Values, v)
	if ambiguous {
		return -1, output.ErrValidation("value %q matches several values of %q ignoring case — use the exact spelling (existing: %s)", v, dim.Name, strings.Join(dim.Values, ","))
	}
	if i < 0 {
		return -1, output.ErrValidation("option %q has no value %q (existing: %s)", dim.Name, v, strings.Join(dim.Values, ","))
	}
	return i, nil
}

func (m *matrix) missingDim(name string) error {
	return output.ErrValidation("option %q does not exist on this product (existing: %s)", name, m.describe())
}

// rename runs one --rename payload against the matrix.
func (m *matrix) rename(s optionSpec) error {
	d, err := m.findDim(s.Name)
	if err != nil {
		return err
	}
	if d >= 0 && m.renamed[m.dims[d].Name] { // names resolve as read, so renames never chain
		d = -1
	}
	if s.NewName != "" {
		return m.renameDimension(d, s.Name, s.NewName)
	}
	if d < 0 {
		return m.missingDim(s.Name)
	}
	return m.renameValues(d, s.Renames)
}

// projections returns the distinct value tuples of the rows with dimension d
// blanked, in row order.
func (m *matrix) projections(d int) [][]string {
	seen := map[string]bool{}
	var out [][]string
	for _, r := range m.rows {
		p := slices.Clone(r.values)
		p[d] = ""
		if k := tupleKey(p); !seen[k] {
			seen[k] = true
			out = append(out, p)
		}
	}
	return out
}

// uses reports whether any row has value v at dimension d.
func (m *matrix) uses(d int, v string) bool {
	return slices.ContainsFunc(m.rows, func(r *row) bool { return r.values[d] == v })
}

// addRowsFor adds a row with value v at dimension d for each existing
// combination of the other dimensions, skipping combinations already present.
func (m *matrix) addRowsFor(d int, v string) {
	have := map[string]bool{}
	for _, r := range m.rows {
		have[tupleKey(r.values)] = true
	}
	for _, p := range m.projections(d) {
		p[d] = v
		if !have[tupleKey(p)] {
			m.rows = append(m.rows, &row{values: p})
		}
	}
}

// addValues appends values to dimension d, each with one new row per existing
// combination of the other dimensions.
func (m *matrix) addValues(d int, values []string) error {
	for _, v := range values {
		if slices.Contains(m.dims[d].Values, v) {
			if m.dims[d].idle[v] { // listed but variantless: give it its variants
				delete(m.dims[d].idle, v)
				m.addRowsFor(d, v)
			}
			continue
		}
		if err := m.rejectCaseTwin(d, v); err != nil {
			return err
		}
		m.addRowsFor(d, v)
		m.dims[d].Values = append(m.dims[d].Values, v)
	}
	return nil
}

// rejectCaseTwin refuses v when it differs only by case from a value of
// dimension d: the two are distinct on the server.
func (m *matrix) rejectCaseTwin(d int, v string) error {
	for _, h := range m.dims[d].Values {
		if foldKey(h) == foldKey(v) {
			return output.ErrValidation("value %q differs from existing %q of %q only by case — use --rename to respell it", v, h, m.dims[d].Name)
		}
	}
	return nil
}

// addDimension appends a dimension: every row takes its first value, and each
// further value gets a sibling copying that row.
func (m *matrix) addDimension(name string, values []string) error {
	if len(m.dims) >= 3 {
		return output.ErrValidation("at most 3 option dimensions are supported (the platform caps variants at option1/2/3), got %d", len(m.dims)+1)
	}
	for i, v := range values {
		for _, w := range values[:i] {
			if foldKey(w) == foldKey(v) {
				return output.ErrValidation("values %q and %q of %q differ only by case — list one of them", w, v, name)
			}
		}
	}
	rows := make([]*row, 0, len(m.rows)*len(values))
	for _, r := range m.rows {
		base := slices.Clone(r.values)
		r.values = append(r.values, values[0])
		rows = append(rows, r)
		from := r.src
		if from == nil {
			from = r.clone
		}
		for _, v := range values[1:] {
			rows = append(rows, &row{values: append(slices.Clone(base), v), clone: from})
		}
	}
	m.rows = rows
	m.dims = append(m.dims, optionDim{Name: name, Values: slices.Clone(values)})
	m.reshaped = true
	return nil
}

// update replaces dimension d's values; new or variantless listed values get
// rows as in addValues, unlisted ones go with their rows.
func (m *matrix) update(d int, values []string) error {
	dim := &m.dims[d]
	respell := map[string]string{} // old spelling → new
	var rest []string
	for _, v := range values {
		if slices.Contains(dim.Values, v) {
			respell[v] = v
		} else {
			rest = append(rest, v)
		}
	}
	// A case-only match pairs with an old value no exact match claimed.
	var free []string
	for _, v := range dim.Values {
		if _, taken := respell[v]; !taken {
			free = append(free, v)
		}
	}
	var added []string
	for _, v := range rest {
		var hits []string
		for _, f := range free {
			if foldKey(f) == foldKey(v) {
				hits = append(hits, f)
			}
		}
		switch {
		case len(hits) > 1:
			return output.ErrValidation("value %q matches several values of %q ignoring case — use the exact spelling (existing: %s)", v, dim.Name, strings.Join(dim.Values, ","))
		case len(hits) == 1:
			if prev, taken := respell[hits[0]]; taken {
				return output.ErrValidation("values %q and %q both match %q of %q ignoring case — use the exact spelling", prev, v, hits[0], dim.Name)
			}
			respell[hits[0]] = v
		default:
			added = append(added, v)
		}
	}
	for _, v := range added {
		for _, w := range values {
			if w != v && foldKey(w) == foldKey(v) {
				return output.ErrValidation("values %q and %q of %q differ only by case — list one of them", w, v, dim.Name)
			}
		}
	}
	for _, r := range m.rows {
		if v, ok := respell[r.values[d]]; ok {
			r.values[d] = v
		}
	}
	dim.moveIdle(respell)
	// New values follow the combinations that exist before the removal.
	for _, v := range values {
		if slices.Contains(added, v) || dim.idle[v] {
			delete(dim.idle, v)
			m.addRowsFor(d, v)
		}
	}
	m.rows = slices.DeleteFunc(m.rows, func(r *row) bool { return !slices.Contains(values, r.values[d]) })
	dim.Values = slices.Clone(values)
	return nil
}

// removeValues drops values from dimension d together with their rows. A
// deletion needs the exact spelling: case-folding could pick a distinct value.
func (m *matrix) removeValues(d int, values []string) error {
	drop := map[string]bool{}
	for _, v := range values {
		i, err := m.findValue(d, v)
		if err != nil {
			return err
		}
		if got := m.dims[d].Values[i]; got != v {
			return output.ErrValidation("option %q has no value %q — did you mean %q? (values are case-sensitive; existing: %s)", m.dims[d].Name, v, got, strings.Join(m.dims[d].Values, ","))
		}
		drop[v] = true
	}
	dim := &m.dims[d]
	if len(drop) == len(dim.Values) {
		return output.ErrValidation("removing every value of %q would delete all its variants — to drop the dimension and keep the variants use --drop %q", dim.Name, dim.Name)
	}
	dim.Values = slices.DeleteFunc(dim.Values, func(v string) bool { return drop[v] })
	m.rows = slices.DeleteFunc(m.rows, func(r *row) bool { return drop[r.values[d]] })
	return nil
}

// removeDimension drops dimension d; the caller has refused merges.
func (m *matrix) removeDimension(d int) {
	for _, r := range m.rows {
		r.values = slices.Delete(r.values, d, d+1)
	}
	m.dims = slices.Delete(m.dims, d, d+1)
	m.reshaped = true
}

// renameValues renames values of dimension d in place; pairs resolve against
// the values as read and may not take an existing name.
func (m *matrix) renameValues(d int, pairs [][2]string) error {
	dim := &m.dims[d]
	before := slices.Clone(dim.Values)
	type change struct {
		i  int
		to string
	}
	var changes []change
	for _, p := range pairs {
		i, ambiguous := lookup(before, p[0])
		if ambiguous {
			_, err := m.findValue(d, p[0])
			return err
		}
		if i < 0 {
			hint := ""
			if slices.Contains(before, p[1]) {
				hint = fmt.Sprintf(" — %q already exists; was it renamed already?", p[1])
			}
			return output.ErrValidation("option %q has no value %q (existing: %s)%s", dim.Name, p[0], strings.Join(before, ","), hint)
		}
		for _, c := range changes {
			if c.i == i {
				return output.ErrValidation("option %q renames %q twice", dim.Name, before[i])
			}
			if foldKey(c.to) == foldKey(p[1]) {
				return output.ErrValidation("option %q renames two values to %q", dim.Name, p[1])
			}
		}
		for j, h := range before {
			switch {
			case j == i || foldKey(h) != foldKey(p[1]):
			case h == p[1]:
				return output.ErrValidation("cannot rename %q to %q: %q already exists in %q — rename or remove it first", before[i], p[1], h, dim.Name)
			default:
				return output.ErrValidation("cannot rename %q to %q: it differs from existing %q of %q only by case", before[i], p[1], h, dim.Name)
			}
		}
		changes = append(changes, change{i, p[1]})
	}
	respell := map[string]string{}
	for _, c := range changes {
		respell[before[c.i]] = c.to
		dim.Values[c.i] = c.to
	}
	for _, r := range m.rows {
		if v, ok := respell[r.values[d]]; ok {
			r.values[d] = v
		}
	}
	dim.moveIdle(respell)
	return nil
}

// renameDimension renames dimension d (resolved from oldName) in place.
func (m *matrix) renameDimension(d int, oldName, newName string) error {
	if d < 0 {
		hint := ""
		if slices.Contains(m.names(), newName) {
			hint = fmt.Sprintf(" — %q already exists; was it renamed already?", newName)
		}
		return output.ErrValidation("option %q does not exist on this product (existing: %s)%s", oldName, m.describe(), hint)
	}
	for i, dim := range m.dims {
		if i != d && foldKey(dim.Name) == foldKey(newName) {
			return output.ErrValidation("cannot rename %q to %q: dimension %q already exists", m.dims[d].Name, newName, dim.Name)
		}
	}
	m.dims[d].Name = newName
	if m.renamed == nil {
		m.renamed = map[string]bool{}
	}
	m.renamed[newName] = true
	return nil
}

// slots renders a row's values as option1..3.
func (r *row) slots() [3]string {
	var s [3]string
	copy(s[:], r.values)
	return s
}

// unchanged reports whether the plan equals the product as read.
func (m *matrix) unchanged(current []optionDim, olds []oldVariant) bool {
	if len(m.dims) != len(current) || len(m.rows) != len(olds) {
		return false
	}
	for i, d := range m.dims {
		if d.Name != current[i].Name || !slices.Equal(d.Values, current[i].Values) {
			return false
		}
	}
	for _, r := range m.rows {
		if r.src == nil || !slices.Equal(r.values, r.read) {
			return false
		}
	}
	return true
}

// checkRows refuses two kept variants on one combination, and a new row the
// server would merge into an old variant's stored options.
func (m *matrix) checkRows(olds []oldVariant) error {
	seen := map[string]*row{}
	for _, r := range m.rows {
		if r.src == nil {
			continue
		}
		k := tupleKey(r.values)
		if prev, dup := seen[k]; dup {
			return output.ErrValidation("variants %s and %s both have options %q — fix the product's variants before editing its options", prev.src.ID, r.src.ID, strings.Join(r.values, "/"))
		}
		seen[k] = r
	}
	stored := map[[3]string]string{}
	for _, ov := range olds {
		stored[ov.Options] = ov.ID
	}
	for _, r := range m.rows {
		if id, hit := stored[r.slots()]; hit && r.src == nil {
			return output.ErrValidation("new variant %q matches the options variant %s still stores — clear that variant's stale options first", strings.Join(r.values, "/"), id)
		}
	}
	return nil
}

// prune drops values that lost their last variant and returns them per
// dimension; values that had none to begin with are left alone.
func (m *matrix) prune() []map[string]any {
	var pruned []map[string]any
	for d := range m.dims {
		used := map[string]bool{}
		for _, r := range m.rows {
			used[r.values[d]] = true
		}
		var gone []string
		m.dims[d].Values = slices.DeleteFunc(m.dims[d].Values, func(v string) bool {
			drop := !used[v] && !m.dims[d].idle[v]
			if drop {
				gone = append(gone, v)
			}
			return drop
		})
		if len(gone) > 0 {
			pruned = append(pruned, map[string]any{"name": m.dims[d].Name, "values": gone})
		}
	}
	return pruned
}

// sortRows orders rows like the storefront: first dimension slowest. Returns
// an error when two new rows share a combination.
func (m *matrix) sortRows() error {
	index := make([]map[string]int, len(m.dims))
	for d, dim := range m.dims {
		index[d] = map[string]int{}
		for i, v := range dim.Values {
			index[d][v] = i
		}
	}
	key := func(r *row) []int {
		k := make([]int, len(r.values))
		for d, v := range r.values {
			k[d] = index[d][v]
		}
		return k
	}
	slices.SortStableFunc(m.rows, func(a, b *row) int { return slices.Compare(key(a), key(b)) })
	for i := 1; i < len(m.rows); i++ {
		a, b := m.rows[i-1], m.rows[i]
		if slices.Equal(a.values, b.values) && (a.src == nil || b.src == nil) {
			return output.ErrInternal("planned two variants for %q", strings.Join(a.values, "/"))
		}
	}
	return nil
}

// ── combined edits ────────────────────────────────────────────────────────────

const maxVariants = 300

// dropSpec is one --drop payload; Keep is the value whose variants survive.
type dropSpec struct {
	Name, Keep string
}

// editPlan is the parsed combined form.
type editPlan struct {
	adds, removes, sets, renames []optionSpec
	drops                        []dropSpec
	existing                     map[string]string // added dimension → value existing variants take
	ov                           matrixOverrides
	// specless: the product had no dimension when read (a fresh +create), so
	// its one variant takes the first value of each added dimension.
	specless bool
}

func parseEditFlags(in common.ExecInput) (*editPlan, error) {
	p := &editPlan{existing: map[string]string{}}
	parseAll := func(flag string, parse func(string) (optionSpec, error)) ([]optionSpec, error) {
		var out []optionSpec
		for _, raw := range in.Flags.GetStringArray(flag) {
			s, err := parse(raw)
			if err != nil {
				return nil, output.ErrValidation("--%s: %s", flag, err.Error())
			}
			out = append(out, s)
		}
		return out, nil
	}
	var err error
	if p.adds, err = parseAll("add", parseValueSpec); err != nil {
		return nil, err
	}
	if p.removes, err = parseAll("remove", parseValueSpec); err != nil {
		return nil, err
	}
	if p.sets, err = parseAll("set", parseValueSpec); err != nil {
		return nil, err
	}
	if p.renames, err = parseAll("rename", parseRenameSpec); err != nil {
		return nil, err
	}
	for _, raw := range in.Flags.GetStringArray("drop") {
		name, keep, _ := strings.Cut(raw, "=")
		name, keep = strings.TrimSpace(name), strings.TrimSpace(keep)
		if name == "" || strings.Contains(raw, ":") {
			return nil, output.ErrValidation(`--drop takes "Dim" or "Dim=value", got %q`, raw)
		}
		p.drops = append(p.drops, dropSpec{Name: name, Keep: keep})
	}
	for _, raw := range in.Flags.GetStringArray("existing") {
		name, v, ok := strings.Cut(raw, ":")
		name, v = strings.TrimSpace(name), strings.TrimSpace(v)
		if !ok || name == "" || v == "" {
			return nil, output.ErrValidation(`--existing takes "Dim:value", got %q`, raw)
		}
		p.existing[name] = v
	}

	if p.ov.newPrice, err = parsePerValue("--new-price", in.Flags.GetStringArray("new-price"), true); err != nil {
		return nil, err
	}
	if p.ov.setPrice, err = parsePerValue("--set-price", in.Flags.GetStringArray("set-price"), true); err != nil {
		return nil, err
	}
	if p.ov.newImage, err = parsePerValue("--new-image", in.Flags.GetStringArray("new-image"), false); err != nil {
		return nil, err
	}
	p.ov.skuTemplate = in.Flags.GetString("sku-template")
	if in.Flags.Changed("set-stock") {
		p.ov.hasStock, p.ov.stock = true, in.Flags.GetInt("set-stock")
	}
	if in.Flags.Changed("new-stock") {
		p.ov.hasNewStock, p.ov.newStock = true, in.Flags.GetInt("new-stock")
	}
	if p.ov.stock < 0 || p.ov.newStock < 0 {
		return nil, output.ErrValidation("--set-stock and --new-stock must be 0 or more")
	}
	if len(p.adds)+len(p.removes)+len(p.sets)+len(p.renames)+len(p.drops) == 0 &&
		!p.ov.setPrice.any() && !p.ov.hasStock && p.ov.skuTemplate == "" {
		return nil, output.ErrValidation("nothing to do: pass --add, --remove, --set, --drop, --rename, --set-price, --set-stock or --sku-template")
	}
	return p, p.checkConflicts()
}

// checkConflicts refuses combinations whose meaning would depend on order.
func (p *editPlan) checkConflicts() error {
	flagsByDim := map[string][]string{}
	note := func(name, flag string) { flagsByDim[foldKey(name)] = append(flagsByDim[foldKey(name)], flag) }
	for _, s := range p.adds {
		note(s.Name, "--add")
	}
	for _, s := range p.removes {
		note(s.Name, "--remove")
	}
	for _, s := range p.sets {
		note(s.Name, "--set")
	}
	for _, d := range p.drops {
		note(d.Name, "--drop")
	}
	for name, flags := range flagsByDim {
		uniq := slices.Compact(slices.Sorted(slices.Values(flags)))
		if len(uniq) > 1 {
			return output.ErrValidation("dimension %q is given to %s together — say it with one of them (--set lists the final values)", name, strings.Join(uniq, " and "))
		}
		if len(flags) > 1 {
			return output.ErrValidation("dimension %q is given to %s twice — list its values in one payload", name, uniq[0])
		}
	}
	for _, r := range p.renames {
		for _, d := range p.drops {
			if foldKey(d.Name) == foldKey(r.Name) {
				return output.ErrValidation("dimension %q is both dropped and renamed", r.Name)
			}
		}
		for _, pair := range r.Renames {
			for _, s := range p.removes {
				if foldKey(s.Name) == foldKey(r.Name) && slices.Contains(s.Values, pair[0]) {
					return output.ErrValidation("value %q of %q is both removed and renamed", pair[0], r.Name)
				}
			}
			for _, s := range p.sets {
				if foldKey(s.Name) == foldKey(r.Name) && !slices.Contains(s.Values, pair[0]) {
					return output.ErrValidation("value %q of %q is renamed but not kept by --set — list its current name in --set", pair[0], r.Name)
				}
			}
		}
	}
	return nil
}

// apply runs the plan in its fixed order, every name resolving as read.
func (p *editPlan) apply(m *matrix) error {
	dim := func(name string) (int, error) {
		d, err := m.findDim(name)
		if err != nil {
			return -1, err
		}
		if d < 0 {
			return -1, m.missingDim(name)
		}
		return d, nil
	}
	for _, s := range p.removes {
		d, err := dim(s.Name)
		if err != nil {
			return err
		}
		if err := m.removeValues(d, s.Values); err != nil {
			return err
		}
		p.ov.changes = append(p.ov.changes, fmt.Sprintf("%s: removed %s", m.dims[d].Name, strings.Join(s.Values, ", ")))
	}
	for _, s := range p.drops {
		if err := p.drop(m, s, dim); err != nil {
			return err
		}
	}
	for _, s := range p.sets {
		d, err := dim(s.Name)
		if err != nil {
			return err
		}
		before := slices.Clone(m.dims[d].Values)
		if err := m.update(d, s.Values); err != nil {
			return err
		}
		p.ov.changes = append(p.ov.changes, fmt.Sprintf("%s: values %s → %s", m.dims[d].Name, strings.Join(before, ", "), strings.Join(s.Values, ", ")))
	}
	// A later --set can strip a value an earlier one listed of its last variant.
	for _, s := range p.sets {
		d, _ := m.findDim(s.Name)
		for _, v := range s.Values {
			if d >= 0 && !m.uses(d, v) {
				delete(m.dims[d].idle, v)
				m.addRowsFor(d, v)
			}
		}
	}
	for _, s := range p.adds {
		d, err := m.findDim(s.Name)
		if err != nil {
			return err
		}
		if d < 0 {
			if err := p.addDimension(m, s); err != nil {
				return err
			}
			continue
		}
		if err := m.addValues(d, s.Values); err != nil {
			return err
		}
		p.ov.changes = append(p.ov.changes, fmt.Sprintf("%s: added %s", m.dims[d].Name, strings.Join(s.Values, ", ")))
	}
	for name := range p.existing {
		if !slices.ContainsFunc(p.adds, func(s optionSpec) bool { return s.Name == name }) {
			return output.ErrValidation("--existing %q names no dimension added by --add", name)
		}
	}
	for _, s := range p.renames {
		if err := m.rename(s); err != nil {
			return err
		}
		if s.NewName != "" {
			p.ov.changes = append(p.ov.changes, fmt.Sprintf("dimension %s renamed to %s", s.Name, s.NewName))
			continue
		}
		for _, pair := range s.Renames {
			p.ov.changes = append(p.ov.changes, fmt.Sprintf("%s: %s renamed to %s", s.Name, pair[0], pair[1]))
		}
	}
	return nil
}

// drop deletes a dimension, first keeping only the variants with Keep when
// given; refused when variants would still merge.
func (p *editPlan) drop(m *matrix, s dropSpec, dim func(string) (int, error)) error {
	d, err := dim(s.Name)
	if err != nil {
		return err
	}
	name := m.dims[d].Name
	if len(m.dims) == 1 {
		return output.ErrValidation("this would remove every dimension and leave the product without variants — delete the product or keep at least one dimension")
	}
	change := fmt.Sprintf("dimension %s dropped", name)
	if s.Keep != "" {
		i, err := m.findValue(d, s.Keep)
		if err != nil {
			return err
		}
		keep := m.dims[d].Values[i]
		before := len(m.rows)
		m.rows = slices.DeleteFunc(m.rows, func(r *row) bool { return r.values[d] != keep })
		m.dims[d].Values = []string{keep}
		change = fmt.Sprintf("dimension %s dropped, keeping the variants with %s (%d other variants deleted)", name, keep, before-len(m.rows))
	}
	seen := map[string]bool{}
	for _, r := range m.rows {
		k := tupleKey(slices.Delete(slices.Clone(r.values), d, d+1))
		if seen[k] {
			return output.ErrValidation("dropping %q would merge variants that differ only by it — say which value's variants to keep: --drop \"%s=<value>\" (values: %s)",
				name, name, strings.Join(m.dims[d].Values, ","))
		}
		seen[k] = true
	}
	m.removeDimension(d)
	p.ov.changes = append(p.ov.changes, change)
	return nil
}

// addDimension adds a dimension; with several values --existing names the one
// existing variants take, and the listed order stays the storefront order.
func (p *editPlan) addDimension(m *matrix, s optionSpec) error {
	values, existing := s.Values, s.Values[0]
	if v, ok := p.existing[s.Name]; ok || (len(values) > 1 && !p.specless) {
		if !ok {
			return output.ErrValidation("new dimension %q has several values — pass --existing \"%s:<value>\" to say which one the existing variants take (values: %s)",
				s.Name, s.Name, strings.Join(values, ","))
		}
		i := slices.Index(values, v)
		if i < 0 {
			return output.ErrValidation("--existing %q is not one of the values added to %q (%s)", v, s.Name, strings.Join(values, ","))
		}
		existing = v
		values = append([]string{v}, slices.Delete(slices.Clone(values), i, i+1)...)
	}
	if err := m.addDimension(s.Name, values); err != nil {
		return err
	}
	m.dims[len(m.dims)-1].Values = slices.Clone(s.Values)
	p.ov.changes = append(p.ov.changes, fmt.Sprintf("dimension %s added (%s); existing variants take %s", s.Name, strings.Join(s.Values, ", "), existing))
	return nil
}

func execEditPlan(ctx context.Context, in common.ExecInput, id string, p *editPlan) (common.ExecResult, error) {
	getPlan, prod, err := readMatrix(ctx, in, id)
	if err != nil {
		return common.ExecResult{}, err
	}
	m := newMatrix(prod.dims, prod.variants)
	p.specless = len(prod.dims) == 0
	if err := p.apply(m); err != nil {
		return common.ExecResult{}, err
	}
	return writeMatrix(ctx, in, id, getPlan, prod, m, p.ov)
}

// ── per-value flags ───────────────────────────────────────────────────────────

// perValue is a flag taking a plain value, a per-value "Dim:v=x,..." form
// keyed on one dimension of the edited matrix, or both (plain = fallback).
type perValue struct {
	flag    string
	isPrice bool
	scalar  string
	dim     string
	pairs   [][2]string
	d       int               // the dimension's index once bound, -1 if none
	byValue map[string]string // value (as on the edited matrix) → raw
}

func newPerValue(flag string) perValue {
	return perValue{flag: flag, d: -1}
}

func parsePerValue(flag string, raws []string, isPrice bool) (perValue, error) {
	p := newPerValue(flag)
	p.isPrice = isPrice
	for _, raw := range raws {
		raw = strings.TrimSpace(raw)
		if raw == "" {
			continue
		}
		if p.plainForm(raw) {
			if p.scalar != "" {
				return p, output.ErrValidation("%s is given two plain values (%q, %q)", flag, p.scalar, raw)
			}
			if isPrice {
				if _, err := parsePrice(flag, raw); err != nil {
					return p, err
				}
			}
			p.scalar = raw
			continue
		}
		if p.dim != "" {
			return p, output.ErrValidation(`%s names one dimension per value form, got %q and %q`, flag, p.dim, raw)
		}
		name, rest, _ := strings.Cut(raw, ":")
		p.dim = strings.TrimSpace(name)
		for pair := range strings.SplitSeq(rest, ",") {
			if strings.TrimSpace(pair) == "" {
				continue
			}
			v, x, ok := strings.Cut(pair, "=")
			v, x = strings.TrimSpace(v), strings.TrimSpace(x)
			if !ok || v == "" || x == "" {
				return p, output.ErrValidation(`%s per value takes "Dim:value=x,...", got %q`, flag, raw)
			}
			if isPrice {
				if _, err := parsePrice(flag, x); err != nil {
					return p, err
				}
			}
			p.pairs = append(p.pairs, [2]string{v, x})
		}
		if p.dim == "" || len(p.pairs) == 0 {
			return p, output.ErrValidation(`%s per value takes "Dim:value=x,...", got %q`, flag, raw)
		}
	}
	return p, nil
}

// plainForm tells a plain value (a number, an image id or URL) from "Dim:v=x".
func (p perValue) plainForm(raw string) bool {
	if p.isPrice {
		return !strings.Contains(raw, ":")
	}
	return !strings.Contains(raw, ":") || strings.HasPrefix(raw, "//") ||
		strings.HasPrefix(raw, "http:") || strings.HasPrefix(raw, "https:")
}

func (p perValue) any() bool { return p.scalar != "" || p.dim != "" }

// bind resolves the per-value form against the edited matrix.
func (p *perValue) bind(m *matrix) error {
	if p.dim == "" {
		return nil
	}
	d, err := m.findDim(p.dim)
	if err != nil {
		return err
	}
	if d < 0 {
		return output.ErrValidation("%s: option %q does not exist after the edit (dimensions: %s)", p.flag, p.dim, m.describe())
	}
	p.d, p.byValue = d, map[string]string{}
	for _, pair := range p.pairs {
		i, err := m.findValue(d, pair[0])
		if err != nil {
			return output.ErrValidation("%s: %s", p.flag, err.Error())
		}
		p.byValue[m.dims[d].Values[i]] = pair[1]
	}
	return nil
}

// forRow returns the raw value that applies to r.
func (p perValue) forRow(r *row) (string, bool) {
	if p.d >= 0 {
		if x, ok := p.byValue[r.values[p.d]]; ok {
			return x, true
		}
	}
	return p.scalar, p.scalar != ""
}

func (p perValue) raws() []string {
	var out []string
	if p.scalar != "" {
		out = append(out, p.scalar)
	}
	for _, x := range p.byValue {
		out = append(out, x)
	}
	return out
}

func (p perValue) String() string {
	parts := make([]string, 0, len(p.pairs)+1)
	for _, pair := range p.pairs {
		parts = append(parts, pair[0]+"="+pair[1])
	}
	s := strings.Join(parts, ", ")
	if p.dim != "" {
		s = p.dim + ": " + s
	}
	if p.scalar != "" {
		if s != "" {
			return s + "; others " + p.scalar
		}
		return p.scalar
	}
	return s
}

func priceValue(raw string) float64 {
	f, _ := strconv.ParseFloat(raw, 64) // validated by parsePerValue
	return f
}

// ── execution ─────────────────────────────────────────────────────────────────

func unusedPriceWarning(flag string) string {
	return flag + " was not used: no new variant needed a price, and it never changes existing prices (use --set-price)"
}

// cloneFields are copied onto a sibling created by adding a dimension; sku,
// barcode and stock identify one physical item and are never copied.
var cloneFields = []string{"price", "compare_at_price", "cost_price", "weight", "weight_unit"}

// readMatrix reads the product (in dry-run too — the full-replace body
// depends on the current matrix).
func readMatrix(ctx context.Context, in common.ExecInput, id string) (common.PlannedRequest, productMatrix, error) {
	getPlan := PlanGet(id)
	getResp, err := common.Send(ctx, in.Client, getPlan)
	if err != nil {
		return getPlan, productMatrix{}, err
	}
	prod := readProductMatrix(getResp)
	for _, d := range prod.dims {
		if len(d.Values) == 0 {
			return getPlan, prod, output.ErrValidation("option %q has no values on this product, so its variants cannot be mapped — fix the product's options first", d.Name)
		}
	}
	return getPlan, prod, nil
}

// writeMatrix validates the edited matrix and writes it in one PUT.
func writeMatrix(ctx context.Context, in common.ExecInput, id string, getPlan common.PlannedRequest,
	prod productMatrix, m *matrix, ov matrixOverrides) (common.ExecResult, error) {
	if len(m.rows) == 0 {
		return common.ExecResult{}, output.ErrValidation("this would delete every variant — delete the product instead, or keep at least one value")
	}
	if err := validateSKUTemplate(ov.skuTemplate, m.dims); err != nil {
		return common.ExecResult{}, err
	}
	for _, p := range []*perValue{&ov.newPrice, &ov.setPrice, &ov.newImage} {
		if err := p.bind(m); err != nil {
			return common.ExecResult{}, err
		}
	}
	if ov.setPrice.any() {
		ov.changes = append(ov.changes, "price set to "+ov.setPrice.String())
	}
	if ov.hasStock {
		ov.changes = append(ov.changes, fmt.Sprintf("stock of every variant set to %d", ov.stock))
	}

	// No effective change and no per-variant override: skip the write.
	if m.unchanged(prod.dims, prod.variants) && ov.skuTemplate == "" && !ov.hasStock && !ov.setPrice.any() {
		summary := matrixSummary(id, m.dims, len(prod.variants))
		summary["dimension_change"] = false
		summary["kept"] = len(prod.variants)
		summary["created"] = 0
		summary["deleted"] = 0
		summary["no_change"] = true
		if ov.newPrice.any() {
			summary["price_applied"] = 0
			summary["warnings"] = []string{unusedPriceWarning(ov.newPrice.flag)}
		}
		if in.DryRun {
			return common.ExecResult{Plans: []common.PlannedRequest{getPlan}, Summary: summary}, nil
		}
		return common.ExecResult{Body: summary}, nil
	}

	pruned := m.prune()
	if err := m.checkRows(prod.variants); err != nil {
		return common.ExecResult{}, err
	}
	if err := m.sortRows(); err != nil {
		return common.ExecResult{}, err
	}
	if len(m.rows) > maxVariants {
		return common.ExecResult{}, output.ErrValidation("the edit would leave %d variants; a product allows at most %d", len(m.rows), maxVariants)
	}
	imageIDs, err := checkNewRows(m, prod, ov)
	if err != nil {
		return common.ExecResult{}, err
	}

	variants := make([]any, len(m.rows))
	kept, cloned, created, priceApplied := 0, 0, 0, 0
	var createdDetail, remapped, stale []map[string]any
	skuOwner := map[string]string{}
	for i, r := range m.rows {
		combo := strings.Join(r.values, "/")
		v := map[string]any{}
		for n, val := range r.values {
			v[fmt.Sprintf("option%d", n+1)] = val
		}
		switch {
		case r.src != nil:
			// The id pins the row; the API merges onto it, so unsent fields survive.
			v["id"] = r.src.ID
			for n := len(r.values); n < 3; n++ {
				if r.src.Options[n] != "" {
					v[fmt.Sprintf("option%d", n+1)] = "" // a dropped slot must be cleared explicitly
				}
			}
			kept++
			if r.src.label() != combo {
				remapped = append(remapped, map[string]any{"id": r.src.ID, "from": r.src.label(), "to": combo})
			}
			if ov.skuTemplate == "" && skuMayBeStale(r.src, r.values) {
				stale = append(stale, map[string]any{"id": r.src.ID, "sku": r.src.SKU, "options": combo})
			}
		case r.clone != nil:
			for _, k := range cloneFields {
				if x, ok := r.clone.raw[k]; ok && x != nil {
					v[k] = x
				}
			}
			if prod.needImage && r.clone.ImageID != "" {
				v["image_id"] = r.clone.ImageID
			}
			cloned++
			createdDetail = append(createdDetail, map[string]any{"options": combo, "copied_from": r.clone.ID})
		default:
			created++
			createdDetail = append(createdDetail, map[string]any{"options": combo})
		}
		if r.src == nil {
			if _, ok := v["price"]; !ok {
				if x, ok := ov.newPrice.forRow(r); ok {
					v["price"] = priceValue(x)
					priceApplied++
				}
			}
			if _, ok := v["image_id"]; !ok && prod.needImage {
				if x, ok := ov.newImage.forRow(r); ok {
					v["image_id"] = imageIDs[x]
				}
			}
			if ov.hasNewStock {
				v["inventory_quantity"] = ov.newStock
			}
		}
		if x, ok := ov.setPrice.forRow(r); ok {
			v["price"] = priceValue(x)
		}
		if ov.skuTemplate != "" {
			sku := renderSKU(ov.skuTemplate, m.dims, r.values)
			if other, dup := skuOwner[sku]; dup {
				return common.ExecResult{}, output.ErrValidation("--sku-template renders %q for both %q and %q — reference every dimension in the template", sku, other, combo)
			}
			skuOwner[sku] = combo
			v["sku"] = sku
		}
		if ov.hasStock {
			v["inventory_quantity"] = ov.stock
		}
		variants[i] = v
	}

	pinned := map[*oldVariant]bool{}
	for _, r := range m.rows {
		if r.src != nil {
			pinned[r.src] = true
		}
	}
	var deletedDetail []map[string]any
	for i := range prod.variants {
		if !pinned[&prod.variants[i]] {
			deletedDetail = append(deletedDetail, prod.variants[i].detail())
		}
	}

	body := map[string]any{"product": map[string]any{
		"options":                  optionsBody(m.dims),
		"variants":                 variants,
		"has_only_default_variant": false,
	}}
	updatePlan := PlanUpdate(id, body)

	summary := matrixSummary(id, m.dims, len(m.rows))
	summary["dimension_change"] = m.reshaped
	summary["kept"] = kept
	summary["created"] = created
	if cloned > 0 {
		summary["cloned"] = cloned
	}
	summary["deleted"] = len(deletedDetail)
	addDetail(summary, "deleted_detail", deletedDetail)
	addDetail(summary, "created_detail", createdDetail)
	addDetail(summary, "remapped_detail", remapped)
	addDetail(summary, "sku_may_be_stale", stale)
	if len(pruned) > 0 {
		summary["pruned_values"] = pruned
	}
	if ov.newPrice.any() {
		summary["price_applied"] = priceApplied
		if priceApplied == 0 {
			summary["warnings"] = []string{unusedPriceWarning(ov.newPrice.flag)}
		}
	}
	if ov.skuTemplate != "" {
		summary["sku_template_applied"] = true
	}
	if ov.hasStock {
		summary["stock_applied"] = ov.stock
	}
	summary["changes"] = append(ov.changes, fmt.Sprintf("variants: %d kept, %d new, %d deleted (%d → %d)",
		kept, cloned+created, len(deletedDetail), len(prod.variants), len(m.rows)))

	if in.DryRun {
		return common.ExecResult{Plans: []common.PlannedRequest{getPlan, updatePlan}, Summary: summary}, nil
	}
	resp, err := common.Send(ctx, in.Client, updatePlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	summary["variants_total"] = respVariantCount(resp, len(m.rows))
	return common.ExecResult{Body: summary}, nil
}

// checkNewRows enforces a price and (where required) an image for new rows
// that have nothing to copy them from, and resolves the image references.
func checkNewRows(m *matrix, prod productMatrix, ov matrixOverrides) (map[string]string, error) {
	var needPrice, needImage []string
	for _, r := range m.rows {
		if r.src != nil {
			continue
		}
		combo := strings.Join(r.values, "/")
		_, hasNew := ov.newPrice.forRow(r)
		_, hasSet := ov.setPrice.forRow(r)
		if (r.clone == nil || r.clone.Price == nil) && !hasNew && !hasSet {
			needPrice = append(needPrice, combo)
		}
		if _, ok := ov.newImage.forRow(r); prod.needImage && (r.clone == nil || r.clone.ImageID == "") && !ok {
			needImage = append(needImage, combo)
		}
	}
	if len(needPrice) > 0 {
		return nil, output.ErrValidation("new variant(s) would be created with no price to copy (e.g. %q) — pass %s to give them one", needPrice[0], ov.newPrice.flag)
	}
	if len(needImage) > 0 {
		return nil, output.ErrValidation("this product requires a variant image and new variant(s) have none to copy (e.g. %q) — pass %s with one of the product's images", needImage[0], ov.newImage.flag)
	}
	if !ov.newImage.any() {
		return nil, nil
	}
	if !prod.needImage {
		return nil, output.ErrValidation("%s applies only to products that require a variant image; this one does not", ov.newImage.flag)
	}
	ids := map[string]string{}
	for _, ref := range ov.newImage.raws() {
		imageID, ok := resolveImage(prod.images, ref)
		if !ok {
			return nil, output.ErrValidation("%s %q is not one of the product's images — add it first with products images create", ov.newImage.flag, ref)
		}
		ids[ref] = imageID
	}
	return ids, nil
}

// resolveImage maps an image id or src onto one of the product's images.
func resolveImage(images []productImage, ref string) (string, bool) {
	norm := func(s string) string {
		return strings.TrimPrefix(strings.TrimPrefix(s, "https:"), "http:")
	}
	for _, im := range images {
		if im.ID == ref || (im.Src != "" && norm(im.Src) == norm(ref)) {
			return im.ID, true
		}
	}
	return "", false
}

// skuMayBeStale reports a kept sku that still names a value the variant no
// longer has (a removed dimension or a renamed value).
func skuMayBeStale(ov *oldVariant, now []string) bool {
	if ov.SKU == "" {
		return false
	}
	isSep := func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsDigit(r) }
	tokens := strings.FieldsFunc(foldKey(ov.SKU), isSep)
	for _, old := range ov.Options {
		if old == "" || slices.ContainsFunc(now, func(v string) bool { return foldKey(v) == foldKey(old) }) {
			continue
		}
		if strings.ContainsFunc(old, isSep) {
			if strings.Contains(foldKey(ov.SKU), foldKey(old)) {
				return true
			}
		} else if slices.Contains(tokens, foldKey(old)) {
			return true
		}
	}
	return false
}

// ── GET-response readers ──────────────────────────────────────────────────────

// productMatrix is what the edit needs from a `products get` response.
type productMatrix struct {
	dims      []optionDim
	variants  []oldVariant
	needImage bool
	images    []productImage
}

type productImage struct {
	ID  string
	Src string
}

// oldVariant is the slice of an existing variant the mapping needs.
type oldVariant struct {
	ID      string
	Options [3]string // option1..option3, "" when absent
	SKU     string
	// Price is the raw JSON value, written back verbatim.
	Price     any
	Inventory *int
	ImageID   string
	raw       map[string]any
}

func (ov oldVariant) optionValue(pos int) string {
	if pos < 1 || pos > 3 {
		return ""
	}
	return ov.Options[pos-1]
}

func (ov oldVariant) label() string {
	parts := []string{}
	for _, v := range ov.Options {
		if v != "" {
			parts = append(parts, v)
		}
	}
	return strings.Join(parts, "/")
}

// detail renders the old variant for the summary's deleted list.
func (ov oldVariant) detail() map[string]any {
	d := map[string]any{"id": ov.ID, "options": ov.label()}
	if ov.SKU != "" {
		d["sku"] = ov.SKU
	}
	if ov.Price != nil {
		d["price"] = ov.Price
	}
	if ov.Inventory != nil {
		d["inventory_quantity"] = *ov.Inventory
	}
	for _, k := range []string{"compare_at_price", "barcode", "weight"} {
		if x, ok := ov.raw[k]; ok && x != nil && x != "" {
			d[k] = x
		}
	}
	if ov.ImageID != "" {
		d["image_id"] = ov.ImageID
	}
	return d
}

// readProductMatrix extracts the option dimensions (ordered by position),
// variants and image settings from a `products get` response.
func readProductMatrix(resp map[string]any) productMatrix {
	var pm productMatrix
	prod, _ := resp["product"].(map[string]any)
	if prod == nil {
		return pm
	}
	pm.needImage, _ = prod["need_variant_image"].(bool)
	if rawImages, ok := prod["images"].([]any); ok {
		for _, ri := range rawImages {
			if m, _ := ri.(map[string]any); m != nil {
				src, _ := m["src"].(string)
				pm.images = append(pm.images, productImage{ID: asString(m["id"]), Src: src})
			}
		}
	}
	if rawOpts, ok := prod["options"].([]any); ok {
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
			pm.dims = append(pm.dims, optionDim{Name: name, Values: values, pos: pos})
		}
		slices.SortStableFunc(pm.dims, func(a, b optionDim) int { return a.pos - b.pos })
	}
	if rawVars, ok := prod["variants"].([]any); ok {
		for _, rv := range rawVars {
			m, _ := rv.(map[string]any)
			if m == nil {
				continue
			}
			ov := oldVariant{raw: m}
			ov.ID = asString(m["id"])
			for i := range 3 {
				if s, ok := m[fmt.Sprintf("option%d", i+1)].(string); ok {
					ov.Options[i] = s
				}
			}
			ov.SKU, _ = m["sku"].(string)
			ov.Price = m["price"]
			if q, ok := asInt(m["inventory_quantity"]); ok {
				ov.Inventory = &q
			}
			ov.ImageID = asString(m["image_id"])
			if ov.ID != "" {
				pm.variants = append(pm.variants, ov)
			}
		}
	}
	// Values a variant uses but the options array omits are appended.
	for i := range pm.dims {
		for _, ov := range pm.variants {
			v := ov.optionValue(pm.dims[i].slot(i + 1))
			if v != "" && !slices.Contains(pm.dims[i].Values, v) {
				pm.dims[i].Values = append(pm.dims[i].Values, v)
			}
		}
	}
	return pm
}

// ── summary helpers ───────────────────────────────────────────────────────────

// detailCap bounds each detail list so the summary stays small even for huge
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
