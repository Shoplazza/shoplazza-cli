package output

import (
	"encoding/json"
	"fmt"
	"io"
	"sort"
	"strings"
	"text/tabwriter"
)

// Format names accepted by PrintFormatted / --format.
const (
	FormatJSON   = "json"
	FormatPretty = "pretty"
	FormatTable  = "table"
)

// PrintFormatted writes v to w using the specified format.
// Supported formats: FormatJSON (default), FormatPretty, FormatTable.
func PrintFormatted(w io.Writer, v any, format string) error {
	switch format {
	case FormatPretty:
		return printPretty(w, v)
	case FormatTable:
		return printTable(w, v)
	default:
		return PrintJSON(w, v)
	}
}

func printPretty(w io.Writer, v any) error {
	switch typed := v.(type) {
	case map[string]any:
		return printMapPretty(w, typed)
	case []any:
		return printSlicePretty(w, typed)
	default:
		_, err := fmt.Fprintln(w, v)
		return err
	}
}

func printMapPretty(w io.Writer, m map[string]any) error {
	if len(m) == 1 {
		for _, v := range m {
			if nested, ok := v.(map[string]any); ok {
				return printFlatPretty(w, nested)
			}
			if list, ok := v.([]any); ok {
				return printSlicePretty(w, list)
			}
		}
	}

	if list, key := extractListKey(m); list != nil {
		if err := printSlicePretty(w, list); err != nil {
			return err
		}
		meta := make(map[string]any, len(m)-1)
		for k, v := range m {
			if k != key {
				meta[k] = v
			}
		}
		if len(meta) > 0 {
			fmt.Fprintln(w)
			return printFlatPretty(w, meta)
		}
		return nil
	}

	return printFlatPretty(w, m)
}

func printFlatPretty(w io.Writer, m map[string]any) error {
	keys := sortedKeys(m)
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	for _, k := range keys {
		fmt.Fprintf(tw, "%s:\t%s\n", k, formatScalar(m[k], 0))
	}
	return tw.Flush()
}

func printSlicePretty(w io.Writer, items []any) error {
	for i, item := range items {
		fmt.Fprintf(w, "--- [%d] ---\n", i+1)
		switch typed := item.(type) {
		case map[string]any:
			if err := printFlatPretty(w, typed); err != nil {
				return err
			}
		default:
			b, _ := json.Marshal(item)
			fmt.Fprintln(w, string(b))
		}
	}
	return nil
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

func printTable(w io.Writer, v any) error {
	switch typed := v.(type) {
	case map[string]any:
		if list, key := extractListKey(typed); list != nil {
			return printListTable(w, list, typed, key)
		}
		if len(typed) == 1 {
			for _, val := range typed {
				if nested, ok := val.(map[string]any); ok {
					return printObjectTable(w, nested)
				}
			}
		}
		return printObjectTable(w, typed)
	case []any:
		return printListTable(w, typed, nil, "")
	default:
		return PrintJSON(w, v)
	}
}

// extractListKey finds the first list-shaped value in a map and returns
// it (as []any) along with its key. Returns nil, "" if none found.
// Recognises []any and []map[string]any (the latter is what code-generated
// payloads like `schema <module>` emit).
func extractListKey(m map[string]any) ([]any, string) {
	for k, v := range m {
		if list, ok := v.([]any); ok {
			return list, k
		}
		if typed, ok := v.([]map[string]any); ok {
			list := make([]any, len(typed))
			for i, item := range typed {
				list[i] = item
			}
			return list, k
		}
	}
	return nil, ""
}

func printListTable(w io.Writer, items []any, parent map[string]any, listKey string) error {
	if len(items) == 0 {
		fmt.Fprintln(w, "(no items)")
		return nil
	}

	headers := listColumnHeaders(items)
	if len(headers) == 0 {
		return PrintJSON(w, items)
	}

	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)

	upper := make([]string, len(headers))
	for i, h := range headers {
		upper[i] = strings.ToUpper(h)
	}
	fmt.Fprintln(tw, strings.Join(upper, "\t"))

	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(tw, strings.Join(seps, "\t"))

	for _, item := range items {
		itemMap, ok := item.(map[string]any)
		if !ok {
			b, _ := json.Marshal(item)
			fmt.Fprintln(tw, string(b))
			continue
		}
		row := make([]string, len(headers))
		for i, h := range headers {
			row[i] = formatScalar(itemMap[h], 60)
		}
		fmt.Fprintln(tw, strings.Join(row, "\t"))
	}

	if parent != nil {
		for _, k := range sortedKeys(parent) {
			if k == listKey {
				continue
			}
			fmt.Fprintf(w, "%s: %s\n", k, formatScalar(parent[k], 60))
		}
	}

	return tw.Flush()
}

func printObjectTable(w io.Writer, m map[string]any) error {
	tw := tabwriter.NewWriter(w, 0, 0, 2, ' ', 0)
	fmt.Fprintln(tw, "KEY\tVALUE")
	fmt.Fprintln(tw, "---\t-----")
	for _, k := range sortedKeys(m) {
		fmt.Fprintf(tw, "%s\t%s\n", k, formatScalar(m[k], 60))
	}
	return tw.Flush()
}

func listColumnHeaders(items []any) []string {
	first, ok := items[0].(map[string]any)
	if !ok {
		return nil
	}
	return sortedKeys(first)
}

func sortedKeys(m map[string]any) []string {
	keys := make([]string, 0, len(m))
	for k := range m {
		keys = append(keys, k)
	}
	sort.Strings(keys)
	return keys
}
