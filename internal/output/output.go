package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"maps"
	"os"
	"regexp"
	"slices"
	"strings"

	"github.com/itchyny/gojq"
)

// PrintJSON writes a JSON payload to the target writer.
// HTML escaping is disabled so user-facing strings like "<store-domain>" render
// as-is instead of "<store-domain>" — CLI output is not HTML.
func PrintJSON(w io.Writer, v any) error {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if err := enc.Encode(v); err != nil {
		return err
	}
	_, err := w.Write(buf.Bytes())
	return err
}

// PrintText writes a plain text line to the target writer.
func PrintText(w io.Writer, msg string) error {
	_, err := fmt.Fprintln(w, msg)
	return err
}

// PrintBody renders metadata or non-API payloads (dry-run summaries, schema
// views, etc.) for the given --format, without the {ok, data} envelope — the
// contents are themselves the answer. nil bodies render as an empty object;
// string bodies write through as a plain line. When jq is non-empty, format
// must be "json" and the expression is evaluated against the body.
func PrintBody(w io.Writer, body any, format, jq string) error {
	if jq != "" {
		if format != FormatJSON {
			return ErrValidation("--jq requires --format json")
		}
		if body == nil {
			body = map[string]any{}
		}
		return applyJQ(w, body, jq)
	}
	if body == nil {
		return PrintFormatted(w, map[string]any{}, format)
	}
	if s, ok := body.(string); ok {
		return PrintText(w, s)
	}
	return PrintFormatted(w, body, format)
}

// PrintAPISuccess writes an HTTP response body wrapped in the success envelope
// {"ok": true, "data": <body>} so scripts can branch on .ok. nil bodies become
// {"ok":true,"data":{}}. pretty/table modes skip the envelope and render the
// raw body via PrintBody. When jq is non-empty, format must be "json" and the
// expression is evaluated against the full envelope.
func PrintAPISuccess(w io.Writer, body any, format, jq string) error {
	if jq != "" && format != FormatJSON {
		return ErrValidation("--jq requires --format json")
	}
	// pretty/table/ndjson/csv render the raw body without the {ok,data} envelope:
	// they are human-, stream-, or export-oriented, not the machine envelope.
	if format == FormatPretty || format == FormatTable || format == FormatNDJSON || format == FormatCSV {
		return PrintBody(w, body, format, "")
	}
	if body == nil {
		body = map[string]any{}
	}
	envelope := map[string]any{"ok": true, "data": body}
	// Agents parse "_notice" to learn the CLI or its skills are stale, without
	// it ever touching stdout data or the pretty/table/ndjson paths above.
	if len(pendingNotice) > 0 {
		envelope["_notice"] = pendingNotice
	}
	if jq != "" {
		failure, err := filterJQ(w, envelope, jq)
		if err != nil || failure == "" {
			return err
		}
		if err := PrintJSON(w, envelope); err != nil {
			return err
		}
		return ErrValidation("%s", failure).WithSubtype(SubtypeJQAfterRequest)
	}
	return PrintFormatted(w, envelope, format)
}

// applyJQ evaluates expr against v (after normalising v through encoding/json
// so gojq sees canonical map[string]any / []any / float64 / string / bool /
// nil) and writes each result to w on its own line.
//
// Rendering:
//   - strings: unquoted (so `--jq '.id'` yields `gid_123`, not `"gid_123"`) —
//     matches `jq -r` / `gh api --jq` so results pipe cleanly into shell
//     variables and loops.
//   - objects/arrays: indented JSON (2-space).
//   - numbers/bools/null: their JSON representation.
func applyJQ(w io.Writer, v any, expr string) error {
	failure, err := filterJQ(w, v, expr)
	if err == nil && failure != "" {
		err = ErrValidation("%s", failure)
	}
	return err
}

// filterJQ writes nothing and returns failure when expr fails at run time.
func filterJQ(w io.Writer, v any, expr string) (failure string, err error) {
	query, err := gojq.Parse(expr)
	if err != nil {
		return "", ErrValidation("invalid --jq expression: %v", err)
	}
	normalised, err := normaliseForJQ(v)
	if err != nil {
		return "", ErrInternal("jq input marshal: %v", err)
	}
	var buf bytes.Buffer
	produced, allNull, jqErr := runJQ(&buf, query, normalised)
	if jqErr != nil {
		return "jq: " + describeJQError(expr, normalised, jqErr), nil
	}
	if _, err := w.Write(buf.Bytes()); err != nil {
		return "", err
	}
	// A filter that selected nothing (no output, or only null) is the classic
	// "why is it null?" confusion — e.g. `.request.path` on a real call, where
	// the envelope is {ok,data} and .request exists only under --dry-run. Nudge
	// the human on stderr; stdout still carries the raw jq result verbatim, so the
	// machine contract is untouched and piped/redirected stderr sees no hint.
	if produced == 0 || allNull {
		writeJQEmptyHint(os.Stderr, expr)
	}
	return "", nil
}

func ValidateJQ(format, expr string) error {
	if expr == "" {
		return nil
	}
	if format != FormatJSON {
		return ErrValidation("--jq requires --format json")
	}
	if _, err := gojq.Parse(expr); err != nil {
		return ErrValidation("invalid --jq expression: %v", err)
	}
	return nil
}

var jqPathStep = regexp.MustCompile(`\.[A-Za-z_][A-Za-z0-9_]*|\.?\[-?\d*\]`)

// describeJQError names the failing step of a plain path such as .a.b[].c.
func describeJQError(expr string, input any, err error) string {
	steps := jqPathStep.FindAllString(expr, -1)
	if len(steps) == 0 || strings.Join(steps, "") != strings.TrimSpace(expr) {
		return err.Error()
	}
	var grand []any
	parents := []any{input}
	for i, step := range steps {
		next, failedOn, failed := runJQStep(parents, step)
		if failed {
			return describeJQStep(jqPath(steps[:i]), failedOn, step, jqPath(steps[:max(i-1, 0)]), grand)
		}
		grand, parents = parents, next
	}
	return err.Error()
}

func runJQStep(values []any, step string) (next []any, failedOn any, failed bool) {
	if strings.HasPrefix(step, "[") {
		step = "." + step
	}
	query, err := gojq.Parse(step)
	if err != nil {
		return nil, nil, false
	}
	for _, v := range values {
		iter := query.Run(v)
		for {
			out, ok := iter.Next()
			if !ok {
				break
			}
			if _, isErr := out.(error); isErr {
				return nil, v, true
			}
			next = append(next, out)
		}
	}
	return next, nil, false
}

func describeJQStep(path string, v any, step, grandPath string, grand []any) string {
	if strings.HasPrefix(step, ".[") {
		step = step[1:]
	}
	switch x := v.(type) {
	case nil:
		for _, g := range grand {
			if m, ok := g.(map[string]any); ok {
				return fmt.Sprintf("%s is null; %s has keys: %s", path, grandPath, jqKeys(m))
			}
		}
		return path + " is null"
	case map[string]any:
		return fmt.Sprintf("%s is an object (keys: %s), it has no %s", path, jqKeys(x), step)
	case []any:
		return fmt.Sprintf("%s is an array, it has no %s", path, step)
	case string:
		return fmt.Sprintf("%s is a string, it has no %s", path, step)
	case bool:
		return fmt.Sprintf("%s is a boolean, it has no %s", path, step)
	}
	return fmt.Sprintf("%s is a number, it has no %s", path, step)
}

func jqPath(steps []string) string {
	if len(steps) == 0 {
		return "."
	}
	return strings.Join(steps, "")
}

func jqKeys(m map[string]any) string {
	keys := slices.Sorted(maps.Keys(m))
	if len(keys) > 20 {
		keys = append(keys[:20], "…")
	}
	return strings.Join(keys, ", ")
}

// runJQ evaluates query against input, writing each result to w. It reports how
// many values were produced and whether every one was null.
func runJQ(w io.Writer, query *gojq.Query, input any) (produced int, allNull bool, err error) {
	allNull = true
	iter := query.Run(input)
	for {
		out, ok := iter.Next()
		if !ok {
			return produced, allNull, nil
		}
		if jqErr, isErr := out.(error); isErr {
			return produced, allNull, jqErr
		}
		produced++
		if out != nil {
			allNull = false
		}
		if werr := writeJQResult(w, out); werr != nil {
			return produced, allNull, werr
		}
	}
}

// writeJQEmptyHint writes the empty-result hint to w only when w is a terminal,
// so machine consumers (piped/redirected stderr, CI) never see it.
func writeJQEmptyHint(w io.Writer, expr string) {
	if !IsTerminal(w) {
		return
	}
	_, _ = io.WriteString(w, jqEmptyHintLine(expr, colorEnabled(w)))
}

// jqEmptyHintLine builds the one-line stderr hint shown when --jq selected
// nothing, pointing at the two common causes: data lives under .data, and
// .request is a --dry-run-only field.
func jqEmptyHintLine(expr string, color bool) string {
	return fmt.Sprintf("%s --jq '%s' matched nothing. Run without --jq to see the shape — response data is under '.data'; '.request' only exists with --dry-run.\n",
		colorDim("hint:", color), expr)
}

func normaliseForJQ(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	dec := json.NewDecoder(bytes.NewReader(b))
	dec.UseNumber()
	var out any
	if err := dec.Decode(&out); err != nil {
		return nil, err
	}
	return out, nil
}

func writeJQResult(w io.Writer, v any) error {
	switch typed := v.(type) {
	case string:
		_, err := fmt.Fprintln(w, typed)
		return err
	case nil:
		_, err := fmt.Fprintln(w, "null")
		return err
	case map[string]any, []any:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		enc.SetIndent("", "  ")
		if err := enc.Encode(typed); err != nil {
			return err
		}
		_, err := w.Write(buf.Bytes())
		return err
	default:
		var buf bytes.Buffer
		enc := json.NewEncoder(&buf)
		enc.SetEscapeHTML(false)
		if err := enc.Encode(typed); err != nil {
			return err
		}
		_, err := w.Write(buf.Bytes())
		return err
	}
}
