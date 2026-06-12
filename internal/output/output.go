package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"

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
	if format == FormatPretty || format == FormatTable {
		return PrintBody(w, body, format, "")
	}
	if body == nil {
		body = map[string]any{}
	}
	envelope := map[string]any{"ok": true, "data": body}
	if jq != "" {
		return applyJQ(w, envelope, jq)
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
	query, err := gojq.Parse(expr)
	if err != nil {
		return ErrValidation("invalid --jq expression: %v", err)
	}
	normalised, err := normaliseForJQ(v)
	if err != nil {
		return ErrInternal("jq input marshal: %v", err)
	}
	iter := query.Run(normalised)
	for {
		out, ok := iter.Next()
		if !ok {
			return nil
		}
		if jqErr, isErr := out.(error); isErr {
			return ErrValidation("jq: %v", jqErr)
		}
		if err := writeJQResult(w, out); err != nil {
			return err
		}
	}
}

func normaliseForJQ(v any) (any, error) {
	b, err := json.Marshal(v)
	if err != nil {
		return nil, err
	}
	var out any
	if err := json.Unmarshal(b, &out); err != nil {
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
