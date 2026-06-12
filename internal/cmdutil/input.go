package cmdutil

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"strings"
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
	if err := json.Unmarshal([]byte(resolved), &result); err != nil {
		return nil, fmt.Errorf("%s invalid format, expected JSON object", label)
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
	if err := json.Unmarshal([]byte(resolved), &body); err != nil {
		return nil, fmt.Errorf("--data invalid JSON format")
	}
	return body, nil
}
