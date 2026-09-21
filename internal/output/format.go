package output

import (
	"encoding/csv"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"unicode/utf8"
)

// Format names accepted by PrintFormatted / --format.
const (
	FormatJSON   = "json"
	FormatPretty = "pretty"
	FormatTable  = "table"
	FormatNDJSON = "ndjson"
	FormatCSV    = "csv"
)

// ANSI attributes used to emphasize keys/headers in pretty & table output. They
// are applied only when the destination is a real terminal (see colorEnabled),
// and always AFTER width padding, so column alignment is never thrown off by the
// zero-display-width escape bytes.
const (
	ansiReset = "\x1b[0m"
	ansiBold  = "\x1b[1m"
	ansiCyan  = "\x1b[36m"
	ansiDim   = "\x1b[2m"
	ansiRed   = "\x1b[31m"
)

// maxFlattenDepth bounds how deep table flattening expands nested objects into
// dot-notation columns before falling back to a compact placeholder.
const maxFlattenDepth = 3

// ValidFormat reports whether s names a supported output format.
func ValidFormat(s string) bool {
	switch s {
	case FormatJSON, FormatPretty, FormatTable, FormatNDJSON, FormatCSV:
		return true
	default:
		return false
	}
}

// PrintFormatted writes v to w using the specified format.
// Supported formats: FormatJSON (default), FormatPretty, FormatTable, FormatNDJSON, FormatCSV.
func PrintFormatted(w io.Writer, v any, format string) error {
	switch format {
	case FormatPretty:
		return printPretty(w, v, colorEnabled(w))
	case FormatTable:
		return printTable(w, v, colorEnabled(w))
	case FormatNDJSON:
		return printNDJSON(w, v)
	case FormatCSV:
		return printCSV(w, v)
	default:
		return PrintJSON(w, v)
	}
}

// colorEnabled reports whether ANSI emphasis should be applied to w: only when w
// is a real terminal and NO_COLOR is unset. A pipe, file, or in-memory buffer
// (agents, CI, tests) gets plain text, so machine consumers never see escapes.
func colorEnabled(w io.Writer) bool {
	if _, ok := os.LookupEnv("NO_COLOR"); ok {
		return false
	}
	f, ok := w.(*os.File)
	return ok && IsTerminal(f)
}

func colorKey(s string, color bool) string {
	if !color {
		return s
	}
	return ansiCyan + s + ansiReset
}

func colorHeader(s string, color bool) string {
	if !color {
		return s
	}
	return ansiBold + s + ansiReset
}

func colorDim(s string, color bool) string {
	if !color {
		return s
	}
	return ansiDim + s + ansiReset
}

// printNDJSON writes one compact JSON object per line. A list-shaped payload
// (an []any, or a map whose sole list value is the records — e.g.
// {"products":[...]}) streams one record per line so large reads don't inflate
// into one giant array that overflows an agent's tool-result limit. Any other
// value is written as a single line.
func printNDJSON(w io.Writer, v any) error {
	switch typed := v.(type) {
	case []any:
		return writeNDJSONLines(w, typed)
	case map[string]any:
		// Prefer the object-list (same choice table/pretty make via dominantObjectList)
		// so all formats stream the same records; fall back to any list.
		if list, _, ok := dominantObjectList(typed); ok {
			return writeNDJSONLines(w, list)
		}
		if list, _ := extractListKey(typed); list != nil {
			return writeNDJSONLines(w, list)
		}
		return writeNDJSONLine(w, typed)
	default:
		return writeNDJSONLine(w, typed)
	}
}

func writeNDJSONLines(w io.Writer, items []any) error {
	for _, item := range items {
		if err := writeNDJSONLine(w, item); err != nil {
			return err
		}
	}
	return nil
}

// writeNDJSONLine writes v as one compact, HTML-unescaped JSON line. Encode
// already appends the trailing newline.
func writeNDJSONLine(w io.Writer, v any) error {
	enc := json.NewEncoder(w)
	enc.SetEscapeHTML(false)
	return enc.Encode(v)
}

// printCSV renders v as CSV (header row + one row per record) for spreadsheet
// export. A list-shaped payload (an []any, or a map whose sole list value is the
// records, e.g. {"products":[...]}) streams one record per row; any other map is
// treated as a single record; a scalar prints as-is. Columns follow the same
// per-domain allow-list as --format table, so exports stay to the useful fields.
func printCSV(w io.Writer, v any) error {
	switch typed := v.(type) {
	case []any:
		return writeCSV(w, "", typed)
	case map[string]any:
		if list, key, ok := dominantObjectList(typed); ok {
			return writeCSV(w, key, list)
		}
		if list, key := extractListKey(typed); list != nil {
			return writeCSV(w, key, list)
		}
		return writeCSV(w, "", []any{typed})
	default:
		_, err := fmt.Fprintln(w, formatScalar(v, 0))
		return err
	}
}

func writeCSV(w io.Writer, listKey string, items []any) error {
	cw := csv.NewWriter(w)
	if len(items) == 0 {
		cw.Flush()
		return cw.Error()
	}
	headers := selectColumns(listKey, items)
	if len(headers) == 0 {
		headers = listColumnHeaders(items)
	}
	// Items that aren't maps (e.g. a list of strings): one value per row, no header.
	if len(headers) == 0 {
		for _, it := range items {
			if err := cw.Write([]string{formatScalar(it, 0)}); err != nil {
				return err
			}
		}
		cw.Flush()
		return cw.Error()
	}
	if err := cw.Write(headers); err != nil {
		return err
	}
	for _, it := range items {
		m, ok := it.(map[string]any)
		if !ok {
			if err := cw.Write([]string{formatScalar(it, 0)}); err != nil {
				return err
			}
			continue
		}
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = csvCell(m[h])
		}
		if err := cw.Write(row); err != nil {
			return err
		}
	}
	cw.Flush()
	return cw.Error()
}

// csvCell renders one CSV cell. Nested objects/arrays are kept as compact JSON
// (not truncated) so an export loses no data; encoding/csv handles quoting.
func csvCell(v any) string {
	switch v.(type) {
	case map[string]any, []any:
		b, _ := json.Marshal(v)
		return string(b)
	default:
		return formatScalar(v, 0)
	}
}

// ── pretty ──────────────────────────────────────────────────────────────────

func printPretty(w io.Writer, v any, color bool) error {
	switch typed := v.(type) {
	case map[string]any:
		return prettyMap(w, typed, color)
	case []any:
		return prettyList(w, typed, nil, 0, color)
	default:
		_, err := fmt.Fprintln(w, formatScalar(v, 0))
		return err
	}
}

func prettyMap(w io.Writer, m map[string]any, color bool) error {
	// Unwrap a single-key wrapper around a nested object: {theme:{...}} → {...}.
	if len(m) == 1 {
		for _, v := range m {
			if nested, ok := v.(map[string]any); ok {
				return prettyMap(w, nested, color)
			}
		}
	}
	// A clean list ENVELOPE ({records:[…]} + scalar meta) → render the list, then
	// the meta. A rich detail object that merely contains a sub-collection (a
	// product with variants[]) is NOT an envelope and renders as an object below.
	if list, key, ok := dominantObjectList(m); ok {
		// Label the list with its key so the reader knows what the [i] items are
		// (e.g. "profiles:"), then indent the items beneath it.
		_, _ = fmt.Fprintf(w, "%s\n", colorKey(key+":", color))
		if err := prettyList(w, list, selectColumns(key, list), 2, color); err != nil {
			return err
		}
		meta := mapWithout(m, key)
		if len(meta) > 0 {
			_, _ = fmt.Fprintln(w)
			return prettyBlock(w, meta, 0, color)
		}
		return nil
	}
	return prettyBlock(w, m, 0, color)
}

// prettyBlock renders an object's fields at the given indent. Scalar (and
// scalar-array) values are aligned within the block; a nested object or an array
// of objects prints its key on its own line and recurses one level deeper.
func prettyBlock(w io.Writer, m map[string]any, indent int, color bool) error {
	keys := orderedKeys(m)
	width := inlineKeyWidth(m, keys)
	prefix := strings.Repeat(" ", indent)
	for _, k := range keys {
		v := m[k]
		if nested, ok := v.(map[string]any); ok {
			if len(nested) == 0 {
				writeKV(w, prefix, k, colorDim("{}", color), width, color)
				continue
			}
			_, _ = fmt.Fprintf(w, "%s%s\n", prefix, colorKey(k+":", color))
			if err := prettyBlock(w, nested, indent+2, color); err != nil {
				return err
			}
			continue
		}
		if list, ok := toAnySlice(v); ok {
			if sok, joined := joinScalars(list); sok {
				writeKV(w, prefix, k, joined, width, color)
				continue
			}
			_, _ = fmt.Fprintf(w, "%s%s\n", prefix, colorKey(k+":", color))
			if err := prettyList(w, list, nil, indent+2, color); err != nil {
				return err
			}
			continue
		}
		writeKV(w, prefix, k, formatScalar(v, 0), width, color)
	}
	return nil
}

// writeKV prints one aligned "key: value" line. The key token is padded to width
// on its raw text, then colorized, so alignment survives the escape bytes.
func writeKV(w io.Writer, prefix, k, val string, width int, color bool) {
	token := k + ":"
	pad := width - utf8.RuneCountInString(token)
	if pad < 0 {
		pad = 0
	}
	_, _ = fmt.Fprintf(w, "%s%s%s %s\n", prefix, colorKey(token, color), strings.Repeat(" ", pad), val)
}

// prettyList renders each item as a dimmed "[i]" header followed by its fields.
// cols, when non-empty, trims each map item to those curated columns.
func prettyList(w io.Writer, items []any, cols []string, indent int, color bool) error {
	prefix := strings.Repeat(" ", indent)
	for i, item := range items {
		header := prefix + colorDim(fmt.Sprintf("[%d]", i+1), color)
		switch t := item.(type) {
		case map[string]any:
			m := t
			if len(cols) > 0 {
				m = filterMap(t, cols)
			}
			_, _ = fmt.Fprintln(w, header)
			if err := prettyBlock(w, m, indent+2, color); err != nil {
				return err
			}
		default:
			_, _ = fmt.Fprintf(w, "%s %s\n", header, formatScalar(item, 0))
		}
	}
	return nil
}

// inlineKeyWidth is the max "key:" token width among the block's fields that
// render on one line (scalars and scalar arrays), for value alignment.
func inlineKeyWidth(m map[string]any, keys []string) int {
	width := 0
	for _, k := range keys {
		inline := true
		v := m[k]
		if nested, ok := v.(map[string]any); ok {
			inline = len(nested) == 0
		} else if list, ok := toAnySlice(v); ok {
			sok, _ := joinScalars(list)
			inline = sok
		}
		if inline {
			if l := utf8.RuneCountInString(k) + 1; l > width {
				width = l
			}
		}
	}
	return width
}

// joinScalars returns (true, "a, b, c") when every item is a scalar, else
// (false, ""). Used to render a scalar array inline instead of as sub-blocks.
func joinScalars(items []any) (bool, string) {
	parts := make([]string, 0, len(items))
	for _, it := range items {
		switch it.(type) {
		case map[string]any, []any:
			return false, ""
		}
		parts = append(parts, formatScalar(it, 0))
	}
	return true, strings.Join(parts, ", ")
}

// formatScalar renders v as a one-line string suitable for tabular/pretty
// output. truncateAt limits the JSON fallback length (0 = no truncation).
func formatScalar(v any, truncateAt int) string {
	switch typed := v.(type) {
	case string:
		return typed
	case bool:
		return fmt.Sprintf("%t", typed)
	case float64:
		if typed == float64(int64(typed)) {
			return fmt.Sprintf("%d", int64(typed))
		}
		return fmt.Sprintf("%g", typed)
	case nil:
		return ""
	default:
		b, _ := json.Marshal(v)
		s := string(b)
		if truncateAt > 0 && len(s) > truncateAt {
			s = s[:truncateAt-3] + "..."
		}
		return s
	}
}

// ── table ───────────────────────────────────────────────────────────────────

func printTable(w io.Writer, v any, color bool) error {
	switch typed := v.(type) {
	case map[string]any:
		if list, key, ok := dominantObjectList(typed); ok {
			return listTable(w, list, typed, key, color)
		}
		if len(typed) == 1 {
			for _, val := range typed {
				if nested, ok := val.(map[string]any); ok {
					return objectTable(w, nested, color)
				}
			}
		}
		return objectTable(w, typed, color)
	case []any:
		return listTable(w, typed, nil, "", color)
	default:
		return PrintJSON(w, v)
	}
}

// listTable renders items as a table. Curated lists (orders/products/…) use the
// allow-listed scalar columns; any other list flattens each record to
// dot-notation columns (http.method, http.path) so nested objects stay visible
// instead of collapsing to "{…}". Parent (non-list) keys print below.
func listTable(w io.Writer, items []any, parent map[string]any, listKey string, color bool) error {
	// Caption the table with the list's key (e.g. "profiles:") so the rows are
	// identifiable; a bare top-level list (no key) gets no caption.
	if listKey != "" {
		_, _ = fmt.Fprintf(w, "%s\n", colorKey(listKey+":", color))
	}
	if len(items) == 0 {
		_, _ = fmt.Fprintln(w, "(no items)")
		return writeParentMeta(w, parent, listKey, color)
	}

	var headers []string
	var rows [][]string

	if cols := selectColumns(listKey, items); len(cols) > 0 {
		headers = cols
		for _, item := range items {
			m, ok := item.(map[string]any)
			if !ok {
				continue
			}
			row := make([]string, len(cols))
			for i, c := range cols {
				row[i] = tableCell(m[c])
			}
			rows = append(rows, row)
		}
	} else {
		headers, rows = flattenRows(items)
	}

	if len(headers) == 0 {
		if err := PrintJSON(w, items); err != nil {
			return err
		}
		return writeParentMeta(w, parent, listKey, color)
	}

	if err := renderTable(w, headers, rows, color); err != nil {
		return err
	}
	return writeParentMeta(w, parent, listKey, color)
}

// flattenRows flattens each map item to dot-notation entries and returns the
// union of columns (first-occurrence order) plus per-item value rows.
func flattenRows(items []any) ([]string, [][]string) {
	var cols []string
	seen := map[string]bool{}
	perItem := make([]map[string]string, len(items))
	for i, item := range items {
		m, ok := item.(map[string]any)
		if !ok {
			continue
		}
		values := map[string]string{}
		for _, e := range flattenEntries(m, "", 0) {
			values[e.Key] = e.Value
			if !seen[e.Key] {
				seen[e.Key] = true
				cols = append(cols, e.Key)
			}
		}
		perItem[i] = values
	}
	rows := make([][]string, 0, len(items))
	for _, values := range perItem {
		if values == nil {
			continue
		}
		row := make([]string, len(cols))
		for i, c := range cols {
			row[i] = values[c]
		}
		rows = append(rows, row)
	}
	return cols, rows
}

type flatEntry struct{ Key, Value string }

// flattenEntries flattens a nested object into ordered dot-notation entries.
// Objects deeper than maxFlattenDepth (and empty/array values) collapse to a
// compact placeholder via tableCell rather than exploding into columns.
func flattenEntries(m map[string]any, prefix string, depth int) []flatEntry {
	var out []flatEntry
	for _, k := range orderedKeys(m) {
		key := k
		if prefix != "" {
			key = prefix + "." + k
		}
		if nested, ok := m[k].(map[string]any); ok && len(nested) > 0 && depth+1 < maxFlattenDepth {
			out = append(out, flattenEntries(nested, key, depth+1)...)
			continue
		}
		out = append(out, flatEntry{Key: key, Value: tableCell(m[k])})
	}
	return out
}

// objectTable renders a single object as a flattened KEY/VALUE table.
func objectTable(w io.Writer, m map[string]any, color bool) error {
	entries := flattenEntries(m, "", 0)
	rows := make([][]string, len(entries))
	for i, e := range entries {
		rows[i] = []string{e.Key, e.Value}
	}
	return renderTable(w, []string{"KEY", "VALUE"}, rows, color)
}

// renderTable lays out headers + rows in aligned columns using manual width
// computation (not tabwriter), so bold headers with ANSI escapes stay aligned.
func renderTable(w io.Writer, headers []string, rows [][]string, color bool) error {
	n := len(headers)
	widths := make([]int, n)
	for i, h := range headers {
		widths[i] = utf8.RuneCountInString(h)
	}
	for _, r := range rows {
		for i := 0; i < n && i < len(r); i++ {
			if l := utf8.RuneCountInString(r[i]); l > widths[i] {
				widths[i] = l
			}
		}
	}
	// Headers render upper-case (table convention); rune count is unchanged, so
	// column widths and the separator row still line up.
	upper := make([]string, n)
	for i, h := range headers {
		upper[i] = strings.ToUpper(h)
	}
	if err := writeTableRow(w, upper, widths, color, true); err != nil {
		return err
	}
	seps := make([]string, n)
	for i := range seps {
		seps[i] = strings.Repeat("-", widths[i])
	}
	if err := writeTableRow(w, seps, widths, false, false); err != nil {
		return err
	}
	for _, r := range rows {
		if err := writeTableRow(w, r, widths, false, false); err != nil {
			return err
		}
	}
	return nil
}

// writeTableRow pads each cell to its column width (on raw text), optionally
// bolds the whole row (headers), and joins with two spaces. Trailing padding is
// trimmed so lines don't carry dangling spaces.
func writeTableRow(w io.Writer, cells []string, widths []int, color, bold bool) error {
	parts := make([]string, len(widths))
	for i := range widths {
		c := ""
		if i < len(cells) {
			c = cells[i]
		}
		pad := widths[i] - utf8.RuneCountInString(c)
		if pad < 0 {
			pad = 0
		}
		cell := c + strings.Repeat(" ", pad)
		if bold {
			cell = colorHeader(cell, color)
		}
		parts[i] = cell
	}
	line := strings.TrimRight(strings.Join(parts, "  "), " ")
	_, err := fmt.Fprintln(w, line)
	return err
}

// writeParentMeta prints a list envelope's non-list keys below the table.
func writeParentMeta(w io.Writer, parent map[string]any, listKey string, color bool) error {
	if parent == nil {
		return nil
	}
	for _, k := range orderedKeys(parent) {
		if k == listKey {
			continue
		}
		_, _ = fmt.Fprintf(w, "%s %s\n", colorKey(k+":", color), tableCell(parent[k]))
	}
	return nil
}

// tableCell renders one list-table cell: a scalar as usual, a scalar array
// joined ("a, b"), an object array as "[N]", and a nested object as "{…}" —
// anything but a wall of truncated JSON.
func tableCell(v any) string {
	if _, ok := v.(map[string]any); ok {
		return "{…}"
	}
	if list, ok := toAnySlice(v); ok {
		if sok, joined := joinScalars(list); sok {
			if utf8.RuneCountInString(joined) > 60 {
				joined = string([]rune(joined)[:57]) + "..."
			}
			return joined
		}
		return fmt.Sprintf("[%d]", len(list))
	}
	return formatScalar(v, 60)
}

// toAnySlice normalizes the slice types this CLI actually emits ([]any from JSON;
// []string and []map[string]any from code-generated payloads) to []any. Non-slice
// values return ok=false.
func toAnySlice(v any) ([]any, bool) {
	switch t := v.(type) {
	case []any:
		return t, true
	case []string:
		out := make([]any, len(t))
		for i, s := range t {
			out[i] = s
		}
		return out, true
	case []map[string]any:
		out := make([]any, len(t))
		for i, m := range t {
			out[i] = m
		}
		return out, true
	default:
		return nil, false
	}
}

// ── shared helpers ──────────────────────────────────────────────────────────

// extractListKey finds the first list-shaped value in a map and returns
// it (as []any) along with its key. Returns nil, "" if none found.
// Recognises []any and []map[string]any (the latter is what code-generated
// payloads like `schema <module>` emit).
func extractListKey(m map[string]any) ([]any, string) {
	// Deterministic: probe keys in priority-then-alpha order so a map with more
	// than one list value always resolves to the same one.
	for _, k := range orderedKeys(m) {
		switch v := m[k].(type) {
		case []any:
			return v, k
		case []map[string]any:
			list := make([]any, len(v))
			for i, item := range v {
				list[i] = item
			}
			return list, k
		}
	}
	return nil, ""
}

// isObjectList reports whether items is a non-empty list whose first element is
// an object — the signal to render it as record sections/rows rather than an
// inline scalar list.
func isObjectList(items []any) bool {
	if len(items) == 0 {
		return false
	}
	_, ok := items[0].(map[string]any)
	return ok
}

// dominantObjectList decides whether m is a clean list envelope — exactly one
// object-list whose siblings are all scalars or scalar arrays (pagination/meta).
// It returns that list and key. A sibling that is itself an object, or a second
// object-list, means m is a rich record (e.g. a product with variants[] and an
// image{}); those render as an object, not as the sole list.
func dominantObjectList(m map[string]any) ([]any, string, bool) {
	var list []any
	var key string
	objLists := 0
	for _, k := range orderedKeys(m) {
		v := m[k]
		if s, ok := toAnySlice(v); ok {
			if isObjectList(s) {
				objLists++
				if list == nil {
					list, key = s, k
				}
			}
			continue // scalar array sibling is allowed as meta
		}
		if _, ok := v.(map[string]any); ok {
			return nil, "", false // a nested object sibling → not an envelope
		}
	}
	if objLists == 1 {
		return list, key, true
	}
	return nil, "", false
}

func listColumnHeaders(items []any) []string {
	first, ok := items[0].(map[string]any)
	if !ok {
		return nil
	}
	return sortedKeys(first)
}

// listColumnAllowlist maps a list's envelope key (the plural resource name,
// e.g. "orders") to a curated, ordered set of columns for --format table/pretty.
// Rich list records carry 40+ fields including nested objects; rendering every
// one produces a table dozens of columns wide with nested objects dumped as
// truncated JSON — unreadable. Only columns actually present in the data are
// shown; an unknown list key, or records matching none of these, fall back to
// flattened top-level keys, so nothing ever renders empty.
var listColumnAllowlist = map[string][]string{
	"orders":    {"id", "number", "status", "financial_status", "fulfillment_status", "total_price", "currency", "placed_at"},
	"products":  {"id", "title", "product_type", "published", "price_min", "inventory_quantity", "updated_at"},
	"customers": {"id", "name", "email", "phone", "orders_count", "total_spent", "created_at"},
	"discounts": {"id", "discount_name", "discount_type", "discount_code", "state", "starts_at", "ends_at"},
}

// selectColumns returns the curated columns for a list under listKey: the
// allow-listed columns present in the first item, in allow-list order. Returns
// nil when there is no allow-list for the key or none of its columns are
// present, signalling the caller to fall back to flattened keys.
func selectColumns(listKey string, items []any) []string {
	allow, ok := listColumnAllowlist[listKey]
	if !ok || len(items) == 0 {
		return nil
	}
	first, ok := items[0].(map[string]any)
	if !ok {
		return nil
	}
	var cols []string
	for _, c := range allow {
		if _, present := first[c]; present {
			cols = append(cols, c)
		}
	}
	return cols
}

// filterMap keeps only the given keys (present ones), for a curated pretty view.
func filterMap(m map[string]any, cols []string) map[string]any {
	out := make(map[string]any, len(cols))
	for _, c := range cols {
		if v, ok := m[c]; ok {
			out[c] = v
		}
	}
	return out
}

// mapWithout returns a copy of m without the given key.
func mapWithout(m map[string]any, drop string) map[string]any {
	out := make(map[string]any, len(m))
	for k, v := range m {
		if k != drop {
			out[k] = v
		}
	}
	return out
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}

// keyPriority ranks well-known identifying fields so they lead in pretty/table
// output ("重点突出") instead of being buried by alphabetical order. Lower =
// earlier; unlisted keys sort alphabetically after all ranked ones.
var keyPriority = map[string]int{
	"id": 0, "name": 1, "title": 1, "handle": 2, "code": 2,
	"status": 3, "state": 3, "type": 4, "email": 5, "phone": 6,
}

// orderedKeys returns m's keys with prioritized fields first, then the rest
// alphabetically — deterministic and emphasis-aware.
func orderedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Slice(keys, func(i, j int) bool {
		pi, iok := keyPriority[keys[i]]
		pj, jok := keyPriority[keys[j]]
		if iok && jok {
			if pi != pj {
				return pi < pj
			}
			return keys[i] < keys[j]
		}
		if iok != jok {
			return iok
		}
		return keys[i] < keys[j]
	})
	return keys
}
