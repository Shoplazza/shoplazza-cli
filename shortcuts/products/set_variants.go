package products

import (
	"context"
	"fmt"
	"slices"
	"strings"
	"unicode"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

// setVariantsShortcut edits a product's option matrix, keeping each variant's
// id wherever it maps onto the new matrix unambiguously.
var setVariantsShortcut = common.Shortcut{
	Service: "products",
	Command: "+set-variants",
	Use:     `+set-variants --id <product-id> --action <add|remove|update|rename> --option "Color:Red,Blue" [--option ...] [--price <n>]`,
	Short:   "Edit a product's option matrix (add/remove/update/rename spec dimensions and values); variants follow",
	Example: `  # Preview adding a Size dimension (existing variants become "S")
  shoplazza products +set-variants --id 12345 --action add --option "Size:S,M,L" --dry-run

  # Add colors to an existing dimension
  shoplazza products +set-variants --id 12345 --action add --option "Color:Red,Blue" --price 24.99

  # Rename a value in place
  shoplazza products +set-variants --id 12345 --action rename --option "Color:Red=Wine Red"`,
	Long: `Edit an existing product's specification matrix in one request: the CLI reads
the product, maps every existing variant onto the new matrix and writes the
full variant list back.

--option names the dimensions/values to operate on and --action says what to
do (exactly one action per call, max 3 dimensions):
  add     append values to a dimension, or add a whole new dimension
  remove  drop values and the variants using them; a bare name ("Size") drops
          the dimension
  update  replace one dimension's value list with exactly the given values
  rename  rename values ("Color:Red=Wine Red,Blue=Navy") or a dimension
          ("Color=Colour") in place
Names and values match exactly; one that differs only by case matches when it
is the only candidate, except that remove needs the exact spelling of a value.
A name or value that does not exist is an error, and a new value that differs
only by case from another of its dimension is refused.

Existing variants keep their id — and with it sku, stock, image and every other
field — except where noted:
  add value         one new variant per existing combination of the other
                    dimensions; combinations deleted earlier stay deleted (a
                    listed value that has no variant yet gets them too)
  add dimension     existing variants take its FIRST value; each other value
                    gets new variants copying price, compare-at and cost
                    price, weight, and the image on products that require one
                    (not sku, barcode or stock), so list the value the current
                    stock belongs to first
  remove value      the variants using it are deleted
  remove dimension  refused when two variants would merge into one
                    combination; remove values first so one is left in each
  update            removed values delete their variants, new values add
                    variants as in "add value"
  rename            values or the name change in place; names resolve as
                    read and may not be taken, so swap two through a
                    temporary name
A value whose last variant goes is dropped from its dimension; values that had
none are left alone. Deleted variants do not break past orders (line items are
snapshotted).

To create a multi-spec product, create it first (products +create), then add
the dimensions here with --action add.

--price prices only new variants that have nothing to copy from; it never
changes an existing price (use products +set-price for that). --image does the
same for products that require a variant image and must be one of the product's
images (products images create adds one).

The output is a bounded summary ({kept, cloned, created, deleted} + details),
never the full product body. --dry-run reads the product and prints the exact
request body it would send, plus the same summary.`,
	DryRunReads: true,
	Flags: []common.Flag{
		{Name: "id", Type: common.FlagString, Required: true, Description: "Product ID (required)."},
		{Name: "action", Type: common.FlagString, Required: true,
			Description: "What to do with the --option payloads: add, remove, update, or rename.",
			Completions: []string{"add", "remove", "update", "rename"}},
		{Name: "option", Type: common.FlagStringArray, Required: true,
			Description: `Dimension payload as "Name:v1,v2,..." (repeat per dimension; values comma-separated, order = storefront order). Bare "Name" only with --action remove; "Name:old=new,..." or "Old=New" with --action rename.`},
		{Name: "price", Type: common.FlagString, Description: "Price for new variants with nothing to copy from (required when such a variant appears). Never changes an existing price."},
		{Name: "image", Type: common.FlagString, Description: "Image (id or URL of one of the product's images) for new variants with nothing to copy from; required when the product requires variant images."},
		{Name: "sku-template", Type: common.FlagString, Description: `SKU pattern with {Option Name} placeholders, e.g. "TS-{Color}-{Size}". Applied to EVERY variant (overwrites existing skus).`},
		{Name: "stock", Type: common.FlagInt, Description: "Inventory quantity set on EVERY variant (absolute set at the default location; overwrites existing stock). Omit to leave stock untouched."},
	},
	Execute: func(ctx context.Context, in common.ExecInput) (common.ExecResult, error) {
		id := strings.TrimSpace(in.Flags.GetString("id"))
		if id == "" {
			return common.ExecResult{}, output.ErrValidation("--id is required")
		}
		action := strings.TrimSpace(in.Flags.GetString("action"))
		switch action {
		case "add", "remove", "update", "rename":
		default:
			return common.ExecResult{}, output.ErrValidation("unknown --action %q: use add, remove, update, or rename", action)
		}

		price, hasPrice, err := parseOptionalPrice(in.Flags.GetString("price"))
		if err != nil {
			return common.ExecResult{}, err
		}
		specs, err := parseOptionSpecs(in.Flags.GetStringArray("option"), action)
		if err != nil {
			return common.ExecResult{}, err
		}
		if in.Flags.Changed("stock") && in.Flags.GetInt("stock") < 0 {
			return common.ExecResult{}, output.ErrValidation("--stock must be 0 or more, got %d", in.Flags.GetInt("stock"))
		}
		return execUpdateMatrix(ctx, in, id, action, specs, matrixOverrides{
			price:       price,
			hasPrice:    hasPrice,
			image:       strings.TrimSpace(in.Flags.GetString("image")),
			skuTemplate: in.Flags.GetString("sku-template"),
			hasStock:    in.Flags.Changed("stock"),
			stock:       in.Flags.GetInt("stock"),
		})
	},
}

// matrixOverrides carries the per-variant flags.
type matrixOverrides struct {
	price       float64
	hasPrice    bool
	image       string
	skuTemplate string
	hasStock    bool
	stock       int
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

// optionSpec is one parsed --option payload.
type optionSpec struct {
	Name    string
	Values  []string    // nil: a bare name (remove only)
	Renames [][2]string // rename: old → new value
	NewName string      // rename: new dimension name
}

// parseOptionSpecs parses the repeated --option payloads for action.
func parseOptionSpecs(specs []string, action string) ([]optionSpec, error) {
	if len(specs) == 0 {
		return nil, output.ErrValidation(`at least one --option "Name:v1,v2,..." is required`)
	}
	out := make([]optionSpec, 0, len(specs))
	seenNames := map[string]bool{}
	for _, raw := range specs {
		var s optionSpec
		var err error
		if action == "rename" {
			s, err = parseRenameSpec(raw)
		} else {
			s, err = parseValueSpec(raw, action == "remove")
		}
		if err != nil {
			return nil, err
		}
		if key := foldKey(s.Name); seenNames[key] {
			return nil, output.ErrValidation("duplicate option name %q", s.Name)
		} else {
			seenNames[key] = true
		}
		out = append(out, s)
	}
	if len(out) > 3 && action != "remove" {
		return nil, output.ErrValidation("at most 3 option dimensions are supported (the platform caps variants at option1/2/3), got %d", len(out))
	}
	return out, nil
}

// parseValueSpec parses `Name:v1,v2`; a bare `Name` is allowed for remove.
func parseValueSpec(spec string, allowBareName bool) (optionSpec, error) {
	name, rawValues, hasColon := strings.Cut(spec, ":")
	name = strings.TrimSpace(name)
	if name == "" {
		return optionSpec{}, output.ErrValidation(`--option must be "Name:v1,v2,...", got %q`, spec)
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
	if len(values) > 0 {
		return optionSpec{Name: name, Values: values}, nil
	}
	if !hasColon && allowBareName {
		return optionSpec{Name: name}, nil
	}
	if !hasColon {
		return optionSpec{}, output.ErrValidation(`--option must be "Name:v1,v2,..." (a bare name is only valid with --action remove), got %q`, spec)
	}
	return optionSpec{}, output.ErrValidation("option %q has no values", name)
}

// parseRenameSpec parses `Name:old=new,...` (values) or `Old=New` (a dimension).
func parseRenameSpec(spec string) (optionSpec, error) {
	name, rawPairs, hasColon := strings.Cut(spec, ":")
	if !hasColon {
		old, newName, ok := strings.Cut(spec, "=")
		old, newName = strings.TrimSpace(old), strings.TrimSpace(newName)
		if !ok || old == "" || newName == "" {
			return optionSpec{}, output.ErrValidation(`--action rename takes "Name:old=new,..." or "OldName=NewName", got %q`, spec)
		}
		return optionSpec{Name: old, NewName: newName}, nil
	}
	name = strings.TrimSpace(name)
	if name == "" {
		return optionSpec{}, output.ErrValidation(`--action rename takes "Name:old=new,..." or "OldName=NewName", got %q`, spec)
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

// parseOptionalPrice parses --price when present. hasPrice=false means the
// flag was omitted (allowed until a new variant actually needs it).
func parseOptionalPrice(s string) (float64, bool, error) {
	if strings.TrimSpace(s) == "" {
		return 0, false, nil
	}
	p, err := parsePrice("--price", s)
	if err != nil {
		return 0, false, err
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

// apply runs one --option payload against the matrix.
func (m *matrix) apply(action string, s optionSpec) error {
	d, err := m.findDim(s.Name)
	if err != nil {
		return err
	}
	switch action {
	case "add":
		if d < 0 {
			return m.addDimension(s.Name, s.Values)
		}
		return m.addValues(d, s.Values)
	case "update":
		if d < 0 {
			return output.ErrValidation("option %q does not exist on this product (existing: %s) — to add a new dimension use --action add", s.Name, m.describe())
		}
		return m.update(d, s.Values)
	case "remove":
		if d < 0 {
			return m.missingDim(s.Name)
		}
		if s.Values == nil {
			return m.removeDimension(d)
		}
		return m.removeValues(d, s.Values)
	case "rename":
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
	return nil
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
			return output.ErrValidation("value %q differs from existing %q of %q only by case — use --action rename to respell it", v, h, m.dims[d].Name)
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
		return output.ErrValidation("removing every value of %q would delete all its variants — to drop the dimension and keep the variants use the bare name --option %q", dim.Name, dim.Name)
	}
	dim.Values = slices.DeleteFunc(dim.Values, func(v string) bool { return drop[v] })
	m.rows = slices.DeleteFunc(m.rows, func(r *row) bool { return drop[r.values[d]] })
	return nil
}

// removeDimension drops dimension d; refused when two rows would merge into
// one combination, since only one of them could keep its data.
func (m *matrix) removeDimension(d int) error {
	dim := m.dims[d]
	if len(m.dims) == 1 {
		return output.ErrValidation("this would remove every dimension and leave the product without variants — delete the product or keep at least one dimension")
	}
	groups := map[string][]*row{}
	var order []string
	for _, r := range m.rows {
		k := tupleKey(slices.Delete(slices.Clone(r.values), d, d+1))
		if _, ok := groups[k]; !ok {
			order = append(order, k)
		}
		groups[k] = append(groups[k], r)
	}
	var merges []string
	for _, k := range order {
		g := groups[k]
		if len(g) < 2 {
			continue
		}
		labels := make([]string, len(g))
		for i, r := range g {
			labels[i] = strings.Join(r.values, "/")
		}
		merges = append(merges, strings.Join(labels, " + "))
	}
	if len(merges) > 0 {
		shown := merges[:min(len(merges), 3)]
		more := ""
		if len(merges) > len(shown) {
			more = fmt.Sprintf(" and %d more", len(merges)-len(shown))
		}
		return output.ErrValidation(
			"removing %q would merge variants into the same combination (%s%s) — first remove the values whose variants should go (--action remove --option \"%s:<value>\") so each combination keeps one variant, then remove the dimension",
			dim.Name, strings.Join(shown, "; "), more, dim.Name)
	}
	for _, r := range m.rows {
		r.values = slices.Delete(r.values, d, d+1)
	}
	m.dims = slices.Delete(m.dims, d, d+1)
	m.reshaped = true
	return nil
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

// ── execution ─────────────────────────────────────────────────────────────────

const unusedPriceWarning = "--price was not used: no new variant needed a price, and it never changes existing prices (use products +set-price)"

// cloneFields are copied onto a sibling created by adding a dimension; sku,
// barcode and stock identify one physical item and are never copied.
var cloneFields = []string{"price", "compare_at_price", "cost_price", "weight", "weight_unit"}

func execUpdateMatrix(ctx context.Context, in common.ExecInput, id, action string, specs []optionSpec, ov matrixOverrides) (common.ExecResult, error) {
	// Read in dry-run too — the full-replace body depends on the current matrix.
	getPlan := PlanGet(id)
	getResp, err := common.Send(ctx, in.Client, getPlan)
	if err != nil {
		return common.ExecResult{}, err
	}
	prod := readProductMatrix(getResp)
	for _, d := range prod.dims {
		if len(d.Values) == 0 {
			return common.ExecResult{}, output.ErrValidation("option %q has no values on this product, so its variants cannot be mapped — fix the product's options first", d.Name)
		}
	}

	m := newMatrix(prod.dims, prod.variants)
	for _, s := range specs {
		if err := m.apply(action, s); err != nil {
			return common.ExecResult{}, err
		}
	}
	if action == "update" {
		// A later payload can strip a value an earlier one listed of its last
		// variant; a listed value always ends up with variants.
		for _, s := range specs {
			d, _ := m.findDim(s.Name)
			for _, v := range s.Values {
				if d >= 0 && !m.uses(d, v) {
					delete(m.dims[d].idle, v)
					m.addRowsFor(d, v)
				}
			}
		}
	}
	if len(m.rows) == 0 {
		return common.ExecResult{}, output.ErrValidation("this would delete every variant — delete the product instead, or keep at least one value")
	}
	if err := validateSKUTemplate(ov.skuTemplate, m.dims); err != nil {
		return common.ExecResult{}, err
	}

	// No effective change and no per-variant override: skip the write.
	if m.unchanged(prod.dims, prod.variants) && ov.skuTemplate == "" && !ov.hasStock {
		summary := matrixSummary(id, m.dims, len(prod.variants))
		summary["action"] = action
		summary["dimension_change"] = false
		summary["kept"] = len(prod.variants)
		summary["created"] = 0
		summary["deleted"] = 0
		summary["no_change"] = true
		if ov.hasPrice {
			summary["price_applied"] = 0
			summary["warnings"] = []string{unusedPriceWarning}
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
	imageID, err := checkNewRows(m, prod, ov)
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
				v["price"] = ov.price
				priceApplied++
			}
			if _, ok := v["image_id"]; !ok && prod.needImage {
				v["image_id"] = imageID
			}
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
	summary["action"] = action
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
	if ov.hasPrice {
		summary["price_applied"] = priceApplied
		if priceApplied == 0 {
			summary["warnings"] = []string{unusedPriceWarning}
		}
	}
	if ov.skuTemplate != "" {
		summary["sku_template_applied"] = true
	}
	if ov.hasStock {
		summary["stock_applied"] = ov.stock
	}

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

// checkNewRows enforces --price and --image for rows that have nothing to copy
// them from, and resolves --image to an image id.
func checkNewRows(m *matrix, prod productMatrix, ov matrixOverrides) (string, error) {
	var needPrice, needImage []string
	for _, r := range m.rows {
		if r.src != nil {
			continue
		}
		combo := strings.Join(r.values, "/")
		if r.clone == nil || r.clone.Price == nil {
			needPrice = append(needPrice, combo)
		}
		if prod.needImage && (r.clone == nil || r.clone.ImageID == "") {
			needImage = append(needImage, combo)
		}
	}
	if len(needPrice) > 0 && !ov.hasPrice {
		return "", output.ErrValidation("new variant(s) would be created with no price to copy (e.g. %q) — pass --price to give them one", needPrice[0])
	}
	if ov.image == "" {
		if len(needImage) > 0 {
			return "", output.ErrValidation("this product requires a variant image and new variant(s) have none to copy (e.g. %q) — pass --image with one of the product's images", needImage[0])
		}
		return "", nil
	}
	if !prod.needImage {
		return "", output.ErrValidation("--image applies only to products that require a variant image; this one does not")
	}
	imageID, ok := resolveImage(prod.images, ov.image)
	if !ok {
		return "", output.ErrValidation("--image %q is not one of the product's images — add it first with products images create", ov.image)
	}
	return imageID, nil
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
