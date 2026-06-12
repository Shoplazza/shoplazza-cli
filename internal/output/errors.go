package output

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"strings"
)

const serverErrorMessage = "The server encountered an internal error. Please contact Shoplazza staff."

// ExitError is a structured error carrying an exit code and an optional
// JSON-serialisable detail block. RunE functions return ExitError (via Errorf /
// ErrWithHint) so the root command can write a JSON envelope to stderr and exit
// with the right code; bare fmt.Errorf breaks the agent-parsable contract.
type ExitError struct {
	Code   int
	Detail *ErrDetail
	Err    error // optional wrapped cause (for errors.Is/As)
}

func (e *ExitError) Error() string {
	if e.Detail != nil {
		return e.Detail.Message
	}
	if e.Err != nil {
		return e.Err.Error()
	}
	return fmt.Sprintf("exit %d", e.Code)
}

func (e *ExitError) Unwrap() error { return e.Err }

// WithField attaches a domain-specific extra field at the top level of the
// "error" JSON object, for payloads that don't fit the well-known schema.
// Returns the receiver for chaining.
func (e *ExitError) WithField(key string, value any) *ExitError {
	if e.Detail == nil {
		e.Detail = &ErrDetail{}
	}
	if e.Detail.Extra == nil {
		e.Detail.Extra = map[string]any{}
	}
	e.Detail.Extra[key] = value
	return e
}

// WithHint sets the envelope hint, returning the receiver for chaining.
func (e *ExitError) WithHint(hint string) *ExitError {
	if e.Detail == nil {
		e.Detail = &ErrDetail{}
	}
	e.Detail.Hint = hint
	return e
}

// WithEndpoint attaches the failing request's method + path to the error's
// detail block. A no-op when both are empty; returns the receiver for chaining.
func (e *ExitError) WithEndpoint(method, path string) *ExitError {
	if method == "" && path == "" {
		return e
	}
	if e.Detail == nil {
		e.Detail = &ErrDetail{}
	}
	if e.Detail.Detail == nil {
		e.Detail.Detail = &ErrorContext{}
	}
	e.Detail.Detail.Method = method
	e.Detail.Detail.Path = path
	return e
}

// Envelope returns the user-facing fields of the error as a map, for test
// introspection; production code uses WriteErrorEnvelope for the wire format.
// Here "code" is the integer exit code (e.Code), distinct from the wire
// format's "code" (the API business-code string ErrDetail.Code).
func (e *ExitError) Envelope() map[string]any {
	out := map[string]any{}
	if e.Detail != nil {
		for k, v := range e.Detail.Extra {
			out[k] = v
		}
		out["type"] = e.Detail.Type
		if e.Detail.Message != "" {
			out["message"] = e.Detail.Message
		}
		if e.Detail.Hint != "" {
			out["hint"] = e.Detail.Hint
		}
	}
	out["code"] = e.Code
	return out
}

// Errorf creates an ExitError with the given exit code, error type, and a
// formatted message. If any argument implements error, it is stored as the
// wrapped cause.
func Errorf(code int, errType, format string, args ...any) *ExitError {
	var wrapped error
	for _, a := range args {
		if e, ok := a.(error); ok {
			wrapped = e
			break
		}
	}
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: fmt.Sprintf(format, args...)},
		Err:    wrapped,
	}
}

// ErrWithHint creates an ExitError with a hint string to guide the user or agent.
func ErrWithHint(code int, errType, msg, hint string) *ExitError {
	return &ExitError{
		Code:   code,
		Detail: &ErrDetail{Type: errType, Message: msg, Hint: hint},
	}
}

// ErrAuth creates an auth-class ExitError (exit code 3).
func ErrAuth(format string, args ...any) *ExitError {
	return Errorf(ExitAuth, TypeAuth, format, args...)
}

// ErrValidation creates a validation-class ExitError (exit code 2).
func ErrValidation(format string, args ...any) *ExitError {
	return Errorf(ExitValidation, TypeValidation, format, args...)
}

// ErrNetwork creates a network-class ExitError (exit code 4).
func ErrNetwork(format string, args ...any) *ExitError {
	return Errorf(ExitNetwork, TypeNetwork, format, args...)
}

// ErrInternal creates an internal-class ExitError (exit code 5). Use for
// unexpected client-side failures — request marshal, URL build, response
// parse — that indicate a CLI bug rather than network or remote trouble.
func ErrInternal(format string, args ...any) *ExitError {
	return Errorf(ExitInternal, TypeInternal, format, args...)
}

// ErrAPI creates an api-class ExitError from a non-2xx HTTP response. Other
// statuses surface the clean server-parsed message (falling back to the raw
// body); 5xx falls back to serverErrorMessage only when the body has none;
// 403 is reclassified to auth-class with a re-login hint. statusCode +
// requestID land in error.detail for triage.
func ErrAPI(statusCode int, body, requestID string) *ExitError {
	code, msg := parseAPIErrorBody(body)
	ctx := newErrorContext(statusCode, requestID)

	if statusCode >= 500 {
		// Surface the server's own 5xx message; fall back to the generic message
		// only when the body carries none.
		if msg == "" {
			msg = serverErrorMessage
		}
		return &ExitError{
			Code:   ExitAPI,
			Detail: &ErrDetail{Type: TypeAPI, Code: code, Message: msg, Detail: ctx},
		}
	}

	if statusCode == 403 {
		if msg == "" {
			msg = "forbidden — your access token lacks the required scope"
		}
		return &ExitError{
			Code: ExitAuth,
			Detail: &ErrDetail{
				Type:    TypeAuth,
				Code:    code,
				Message: msg,
				Hint:    "Run 'shoplazza auth login' to re-authenticate. Use --domain or --scope to request the needed permission.",
				Detail:  ctx,
			},
		}
	}

	if msg == "" {
		if msg = strings.TrimSpace(body); msg == "" {
			msg = fmt.Sprintf("request failed with status %d", statusCode)
		}
	}
	return &ExitError{
		Code:   ExitAPI,
		Detail: &ErrDetail{Type: TypeAPI, Code: code, Message: msg, Detail: ctx},
	}
}

// ErrAPIAuthHint builds an auth-class ExitError from a non-2xx auth/token
// exchange response: like ErrAPI but keeping the auth type and a caller-supplied
// recovery hint alongside the clean server-parsed message + code + status_code.
func ErrAPIAuthHint(statusCode int, body, hint string) *ExitError {
	code, msg := parseAPIErrorBody(body)
	if msg == "" {
		if msg = strings.TrimSpace(body); msg == "" {
			msg = fmt.Sprintf("request failed with status %d", statusCode)
		}
	}
	return &ExitError{
		Code: ExitAuth,
		Detail: &ErrDetail{
			Type:    TypeAuth,
			Code:    code,
			Message: msg,
			Hint:    hint,
			Detail:  newErrorContext(statusCode, ""),
		},
	}
}

func newErrorContext(statusCode int, requestID string) *ErrorContext {
	if statusCode == 0 && requestID == "" {
		return nil
	}
	return &ErrorContext{StatusCode: statusCode, RequestID: requestID}
}

// parseAPIErrorBody pulls a clean human-readable message out of a server error
// body. It recognizes {code,message}, {message}, {error:"..."}, and
// {errors:["a","b"]}, returning the first that's populated. Empty message when
// none matches (callers fall back to the raw body).
func parseAPIErrorBody(body string) (code, message string) {
	var env struct {
		Code    string   `json:"code"`
		Message string   `json:"message"`
		Error   string   `json:"error"`
		Errors  []string `json:"errors"`
	}
	if err := json.Unmarshal([]byte(body), &env); err == nil {
		switch {
		case env.Message != "":
			return env.Code, env.Message
		case env.Error != "":
			return env.Code, env.Error
		case len(env.Errors) > 0:
			return env.Code, strings.Join(env.Errors, "; ")
		}
		return env.Code, ""
	}
	// Some endpoints return a bare JSON string body (e.g. "Unauthorized").
	// Unwrap it so the message doesn't surface with literal surrounding quotes.
	// Non-JSON plain text falls through to the caller's raw-body fallback.
	var str string
	if err := json.Unmarshal([]byte(body), &str); err == nil && str != "" {
		return "", str
	}
	return "", ""
}

// WriteErrorEnvelope serialises err as a JSON ErrorEnvelope and writes it to w.
// A trailing newline is always written. No-op when err.Detail is nil.
func WriteErrorEnvelope(w io.Writer, err *ExitError) {
	if err == nil || err.Detail == nil {
		return
	}
	env := &ErrorEnvelope{OK: false, Error: err.Detail}
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	enc.SetIndent("", "  ")
	if encErr := enc.Encode(env); encErr != nil {
		// Last-resort fallback — should never happen.
		fmt.Fprintf(w, `{"ok":false,"error":{"type":%q,"message":%q}}`+"\n",
			err.Detail.Type, err.Detail.Message)
		return
	}
	_, _ = buf.WriteTo(w)
}
