package common

import (
	"fmt"
	"strconv"
	"strings"
)

// Layer is a single discount condition→obtain pair.
type Layer struct {
	ConditionValue float64
	ObtainValue    float64
}

// fmtLayerVal formats a numeric layer value as the string the API expects.
// e.g. 100.0 → "100", 10.5 → "10.5"
func fmtLayerVal(f float64) string {
	return strconv.FormatFloat(f, 'f', -1, 64)
}

// parseTierVal parses a tier numeric value, tolerating an optional trailing "%"
// so help text promising "discount%" works literally (e.g. "50%" → 50).
func parseTierVal(s string) (float64, error) {
	s = strings.TrimSpace(s)
	s = strings.TrimSuffix(s, "%")
	s = strings.TrimSpace(s)
	return strconv.ParseFloat(s, 64)
}

// ParseTiers parses "100:10,200:25" into ordered Layers.
// Each segment is "condition:obtain".
func ParseTiers(s string) ([]Layer, error) {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil, fmt.Errorf("--tiers must not be empty")
	}
	segments := strings.Split(s, ",")
	layers := make([]Layer, 0, len(segments))
	for _, seg := range segments {
		seg = strings.TrimSpace(seg)
		parts := strings.SplitN(seg, ":", 2)
		if len(parts) != 2 {
			return nil, fmt.Errorf("invalid tier %q: expected format <condition>:<obtain>", seg)
		}
		cond, err := parseTierVal(parts[0])
		if err != nil {
			return nil, fmt.Errorf("invalid tier condition %q: %w", parts[0], err)
		}
		obtain, err := parseTierVal(parts[1])
		if err != nil {
			return nil, fmt.Errorf("invalid tier obtain %q: %w", parts[1], err)
		}
		layers = append(layers, Layer{ConditionValue: cond, ObtainValue: obtain})
	}
	return layers, nil
}

// LayersToMaps converts Layers to the API-compatible []any of map[string]any.
// The API expects condition_value and obtain_value as strings (e.g. "100", "10.5").
func LayersToMaps(layers []Layer) []any {
	out := make([]any, len(layers))
	for i, l := range layers {
		out[i] = map[string]any{
			"condition_value": fmtLayerVal(l.ConditionValue),
			"obtain_value":    fmtLayerVal(l.ObtainValue),
		}
	}
	return out
}

// ParseProducts splits a comma-separated product ID list. "all" or "" returns nil.
func ParseProducts(s string) []string {
	s = strings.TrimSpace(s)
	if s == "" || s == "all" {
		return nil
	}
	parts := strings.Split(s, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			out = append(out, p)
		}
	}
	return out
}
