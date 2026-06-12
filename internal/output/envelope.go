package output

import (
	"bytes"
	"encoding/json"
)

// ErrorEnvelope is the standard JSON error wrapper written to stderr.
// Agents parse this to extract structured error information.
type ErrorEnvelope struct {
	OK    bool       `json:"ok"`
	Error *ErrDetail `json:"error"`
}

// ErrDetail describes a structured error inside an ErrorEnvelope. The typed
// fields plus any Extra entries are merged into one JSON object via
// MarshalJSON, so the well-known fields are tagged json:"-" to avoid
// double-emitting. Extra lets domain helpers attach extra top-level fields
// (task, elapsed_seconds, ...) without bloating this struct.
type ErrDetail struct {
	Type    string         `json:"-"`
	Code    string         `json:"-"`
	Message string         `json:"-"`
	Hint    string         `json:"-"`
	Detail  *ErrorContext  `json:"-"`
	Extra   map[string]any `json:"-"`
}

// MarshalJSON emits the typed fields plus any Extra entries at the top level
// of the error object, with omitempty semantics for code/hint/detail.
func (d *ErrDetail) MarshalJSON() ([]byte, error) {
	if d == nil {
		return []byte("null"), nil
	}
	out := make(map[string]any, len(d.Extra)+5)
	for k, v := range d.Extra {
		out[k] = v
	}
	// Well-known fields overwrite any colliding Extra key — Extra is
	// intended for domain-specific additions, not to override the schema.
	out["type"] = d.Type
	if d.Code != "" {
		out["code"] = d.Code
	}
	out["message"] = d.Message
	if d.Hint != "" {
		out["hint"] = d.Hint
	}
	if d.Detail != nil {
		out["detail"] = d.Detail
	}
	// Encode without HTML-escaping so '<', '>', '&' in messages/hints render
	// literally. The outer encoder can't un-escape a Marshaler's output, so a
	// plain json.Marshal here would leak escapes through.
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(out); err != nil {
		return nil, err
	}
	return bytes.TrimRight(buf.Bytes(), "\n"), nil // Encode appends a newline; MarshalJSON must not
}

// ErrorContext carries operational metadata about a failed HTTP call. Populated
// for api/auth errors that originated from a server response; nil for
// validation / network / internal errors that don't have an HTTP context.
type ErrorContext struct {
	StatusCode int    `json:"status_code,omitempty"`
	RequestID  string `json:"request_id,omitempty"`
	// Method and Path name the failing request (resolved path, no query) so a
	// server error self-identifies which endpoint produced it.
	Method string `json:"method,omitempty"`
	Path   string `json:"path,omitempty"`
}
