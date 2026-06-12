package cmdutil

import "strings"

// EnsureObject returns an existing nested object or creates one.
func EnsureObject(target map[string]any, key string) map[string]any {
	if existing, ok := target[key].(map[string]any); ok {
		return existing
	}
	child := map[string]any{}
	target[key] = child
	return child
}

// AddString inserts a non-empty string into the target map.
func AddString(target map[string]any, key, value string) {
	if strings.TrimSpace(value) != "" {
		target[key] = value
	}
}

// AddSlice inserts a non-empty slice into the target map.
func AddSlice(target map[string]any, key string, values []string) {
	if len(values) > 0 {
		target[key] = values
	}
}
