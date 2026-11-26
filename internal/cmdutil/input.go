package cmdutil

import (
	"bytes"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"strings"
	"unicode/utf8"
)

// ResolveInput resolves special input conventions for raw JSON flags.
// Supported forms:
//   - "-"      => read from stdin
//   - "@file"  => read from file
//   - "'...'"  => strip surrounding single quotes
func ResolveInput(raw string, stdin io.Reader) (string, error) {
	if raw == "" {
		return "", nil
	}

	if raw == "-" {
		if stdin == nil {
			return "", fmt.Errorf("stdin is not available")
		}
		data, err := io.ReadAll(stdin)
		if err != nil {
			return "", fmt.Errorf("failed to read stdin: %w", err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			return "", fmt.Errorf("stdin is empty (did you forget to pipe input?)")
		}
		return s, nil
	}

	if strings.HasPrefix(raw, "@") {
		path := strings.TrimSpace(strings.TrimPrefix(raw, "@"))
		if path == "" {
			return "", fmt.Errorf("file path is empty")
		}
		data, err := os.ReadFile(path)
		if err != nil {
			return "", fmt.Errorf("failed to read file %q: %w", path, err)
		}
		s := strings.TrimSpace(string(data))
		if s == "" {
			return "", fmt.Errorf("file %q is empty", path)
		}
		return s, nil
	}

	if len(raw) >= 2 && raw[0] == '\'' && raw[len(raw)-1] == '\'' {
		raw = raw[1 : len(raw)-1]
	}

	return raw, nil
}

// DecodeJSON is json.Unmarshal with numbers kept as json.Number, so an integer
// past 2^53 is re-encoded digit for digit.
func DecodeJSON(data []byte, v any) error {
	dec := json.NewDecoder(bytes.NewReader(data))
	dec.UseNumber()
	if err := dec.Decode(v); err != nil {
		return err
	}
	end := dec.InputOffset()
	if _, err := dec.Token(); err != io.EOF {
		for end < int64(len(data)) && strings.ContainsRune(" \t\r\n", rune(data[end])) {
			end++
		}
		return &trailingDataError{offset: end}
	}
	return nil
}

type trailingDataError struct{ offset int64 }

func (e *trailingDataError) Error() string { return "invalid character after top-level value" }

// JSONInputError describes a DecodeJSON error on raw; want is "object" or "array".
func JSONInputError(flag string, raw []byte, err error, want string) error {
	var syn *json.SyntaxError
	var typ *json.UnmarshalTypeError
	var trail *trailingDataError
	switch {
	case errors.Is(err, io.EOF):
		return fmt.Errorf("%s is empty", flag)
	case errors.Is(err, io.ErrUnexpectedEOF):
		return fmt.Errorf("%s is not valid JSON %s: a closing } or ] is missing", flag, want)
	case errors.As(err, &trail):
		next, _ := utf8.DecodeRune(raw[trail.offset:])
		return fmt.Errorf("%s is not valid JSON %s: unexpected %q after the end of the %s (%s)",
			flag, want, next, topLevelKind(raw), jsonPosition(raw, trail.offset))
	case errors.As(err, &syn):
		return fmt.Errorf("%s is not valid JSON %s: %s (%s)", flag, want, syn.Error(), jsonPosition(raw, syn.Offset-1))
	case errors.As(err, &typ) && typ.Field == "":
		return fmt.Errorf("%s must be a JSON %s, got %s", flag, want, withArticle(typ.Value))
	}
	return fmt.Errorf("%s: %v", flag, err)
}

func jsonPosition(raw []byte, offset int64) string {
	offset = max(0, min(offset, int64(len(raw))))
	before := raw[:offset]
	line := bytes.Count(before, []byte("\n")) + 1
	column := len(before) - bytes.LastIndexByte(before, '\n')
	return fmt.Sprintf("line %d, column %d", line, column)
}

func topLevelKind(raw []byte) string {
	switch t := bytes.TrimSpace(raw); {
	case len(t) > 0 && t[0] == '{':
		return "object"
	case len(t) > 0 && t[0] == '[':
		return "array"
	}
	return "value"
}

func withArticle(jsonType string) string {
	if jsonType == "bool" {
		return "true or false"
	}
	if strings.ContainsRune("aeiou", rune(jsonType[0])) {
		return "an " + jsonType
	}
	return "a " + jsonType
}

// ParseJSONMap parses a JSON object from raw input.
func ParseJSONMap(input, label string, stdin io.Reader) (map[string]any, error) {
	resolved, err := ResolveInput(input, stdin)
	if err != nil {
		return nil, fmt.Errorf("%s: %w", label, err)
	}
	if resolved == "" {
		return map[string]any{}, nil
	}
	var result map[string]any
	if err := DecodeJSON([]byte(resolved), &result); err != nil {
		return nil, JSONInputError(label, []byte(resolved), err, "object")
	}
	return result, nil
}

// ParseOptionalBody parses a JSON body for methods that support a body.
func ParseOptionalBody(httpMethod, data string, stdin io.Reader) (any, error) {
	switch strings.ToUpper(httpMethod) {
	case "POST", "PUT", "PATCH", "DELETE":
	default:
		return nil, nil
	}
	resolved, err := ResolveInput(data, stdin)
	if err != nil {
		return nil, fmt.Errorf("--data: %w", err)
	}
	if resolved == "" {
		return nil, nil
	}
	var body any
	if err := DecodeJSON([]byte(resolved), &body); err != nil {
		return nil, JSONInputError("--data", []byte(resolved), err, "object")
	}
	return body, nil
}
