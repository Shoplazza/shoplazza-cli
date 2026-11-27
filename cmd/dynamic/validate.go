package dynamic

import (
	"bytes"
	"encoding/json"
	"fmt"
	"maps"
	"slices"
	"sort"
	"strconv"
	"strings"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

// checkParamKeys rejects --params keys that are neither a declared parameter
// nor a {var} of the path template. A key that is a body field gets its own
// subtype, so a caller knows to move it into --data rather than fix a typo.
func checkParamKeys(c registry.Command, params map[string]any, schemaHint string) error {
	declared := map[string]bool{}
	for _, p := range c.Parameters {
		declared[p.Name] = true
	}
	for _, name := range pathParamNames(c.HTTP.Path) {
		declared[name] = true
	}
	bodyField := map[string]bool{}
	if c.Body != nil {
		for _, f := range c.Body.Fields {
			bodyField[f.Name] = true
		}
	}
	var misplaced, unknown []string
	for k := range params {
		switch {
		case declared[k]:
		case bodyField[k]:
			misplaced = append(misplaced, k)
		default:
			unknown = append(unknown, k)
		}
	}
	if len(misplaced) > 0 {
		sort.Strings(misplaced)
		return output.ErrValidation("%s %s of this command, not %s", strings.Join(misplaced, ", "),
			plural(misplaced, "is a request-body field", "are request-body fields"),
			plural(misplaced, "a path/query parameter", "path/query parameters")).
			WithParam(misplaced[0]).
			WithSubtype(output.SubtypeMisplacedParam).
			WithHint("move " + plural(misplaced, "it", "them") + " from --params into --data")
	}
	if len(unknown) > 0 {
		sort.Strings(unknown)
		accepted := make([]string, 0, len(declared))
		for k := range declared {
			accepted = append(accepted, k)
		}
		sort.Strings(accepted)
		hint := "this command takes no --params"
		if len(accepted) > 0 {
			hint = "accepted --params keys: " + strings.Join(accepted, ", ")
		}
		return output.ErrValidation("unknown --params %s: %s", plural(unknown, "key", "keys"), strings.Join(unknown, ", ")).
			WithParam(unknown[0]).
			WithSubtype(output.SubtypeUnknownParam).
			WithHint(hint + "; " + schemaHint + " for details")
	}
	return nil
}

func splitArrayParams(c registry.Command, params map[string]any) {
	for _, p := range c.Parameters {
		if p.In != "query" || p.Type != "array" {
			continue
		}
		s, ok := params[p.Name].(string)
		if !ok || !strings.Contains(s, ",") {
			continue
		}
		items := []any{}
		for _, part := range strings.Split(s, ",") {
			if part = strings.TrimSpace(part); part != "" {
				items = append(items, part)
			}
		}
		params[p.Name] = items
	}
}

// checkBodyWrapper rejects top-level --data keys that belong inside a body object field.
func checkBodyWrapper(c registry.Command, spec *registry.Spec, body any, schemaHint string) error {
	m, _ := body.(map[string]any)
	if c.Body == nil || spec == nil || len(m) == 0 {
		return nil
	}
	top := map[string]bool{}
	for _, f := range c.Body.Fields {
		top[f.Name] = true
	}
	owner := map[string]string{}
	for k := range m {
		if top[k] {
			continue
		}
		var hits []string
		for _, f := range c.Body.Fields {
			if f.Type == "object" && slices.ContainsFunc(spec.Schemas[f.Schema].Fields, func(sf registry.Field) bool { return sf.Name == k }) {
				hits = append(hits, f.Name)
			}
		}
		if len(hits) == 1 {
			owner[k] = hits[0]
		}
	}
	if len(owner) == 0 {
		return nil
	}
	fixed := maps.Clone(m)
	for k, w := range owner {
		inner := map[string]any{}
		if v, ok := fixed[w]; ok {
			given, isMap := v.(map[string]any)
			if !isMap {
				return nil
			}
			inner = maps.Clone(given)
		}
		inner[k] = fixed[k]
		fixed[w] = inner
		delete(fixed, k)
	}
	wrappers := slices.Sorted(maps.Values(owner))
	wrappers = slices.Compact(wrappers)
	quoted := make([]string, len(wrappers))
	for i, w := range wrappers {
		quoted[i] = strconv.Quote(w)
	}
	return output.ErrValidation("--data is missing the %s %s: %s", strings.Join(quoted, ", "),
		plural(wrappers, "wrapper", "wrappers"), renderBody(fixed)).
		WithParam(wrappers[0]).
		WithSubtype(output.SubtypeMissingRequiredField).
		WithHint(schemaHint + " to see the body fields")
}

func renderBody(m map[string]any) string {
	var buf bytes.Buffer
	enc := json.NewEncoder(&buf)
	enc.SetEscapeHTML(false)
	if err := enc.Encode(m); err == nil && buf.Len() <= 200 {
		return strings.TrimSpace(buf.String())
	}
	return bodyKeys(m)
}

func bodyKeys(m map[string]any) string {
	keys := slices.Sorted(maps.Keys(m))
	parts := make([]string, len(keys))
	for i, k := range keys {
		v := "…"
		if inner, ok := m[k].(map[string]any); ok {
			v = bodyKeys(inner)
		}
		parts[i] = strconv.Quote(k) + ":" + v
	}
	return "{" + strings.Join(parts, ",") + "}"
}

// checkRequiredBody rejects a request whose --data lacks a required top-level body field.
func checkRequiredBody(c registry.Command, body any, schemaHint string) error {
	if c.Body == nil || !commandHasBody(c) {
		return nil
	}
	m, _ := body.(map[string]any)
	var missing []string
	for _, f := range c.Body.Fields {
		if v, ok := m[f.Name]; f.Required && (!ok || v == nil) {
			missing = append(missing, f.Name)
		}
	}
	if len(missing) == 0 {
		return nil
	}
	return output.ErrValidation("--data is missing required %s: %s", plural(missing, "field", "fields"), strings.Join(missing, ", ")).
		WithParam(missing[0]).
		WithSubtype(output.SubtypeMissingRequiredField).
		WithHint(schemaHint + " to see the body fields")
}

// checkRequiredParams reports, in one error, every path {var} and required query
// parameter left absent, empty, or as an unfilled placeholder.
func checkRequiredParams(c registry.Command, params map[string]any, binder *paramBinder, schemaHint string) error {
	required := pathParamNames(c.HTTP.Path)
	for _, p := range c.Parameters {
		if p.In == "query" && p.Required {
			required = append(required, p.Name)
		}
	}
	var names, listed, set []string
	flagged := true
	for _, name := range required {
		v, ok := params[name]
		list, isList := v.([]any)
		s, isString := v.(string)
		switch {
		case !ok || v == nil || (isString && strings.TrimSpace(s) == "") || (isList && len(list) == 0):
			listed = append(listed, name)
		case isString && isPlaceholder(s):
			listed = append(listed, fmt.Sprintf("%s (placeholder %q)", name, s))
		default:
			continue
		}
		names = append(names, name)
		if flag := binder.flagFor(name); flag != "" {
			set = append(set, flag)
		} else {
			flagged = false
			set = append(set, fmt.Sprintf("--params '{%q: \"<value>\"}'", name))
		}
	}
	if len(names) == 0 {
		return nil
	}
	hint := "set " + strings.Join(set, ", ")
	if flagged {
		hint += " (or the same " + plural(names, "key", "keys") + " in --params)"
	}
	return output.ErrValidation("missing required %s: %s", plural(names, "parameter", "parameters"), strings.Join(listed, ", ")).
		WithParam(names[0]).
		WithSubtype(output.SubtypeMissingRequiredParam).
		WithHint(hint + "; " + schemaHint + " to see required parameters")
}

func plural(items []string, one, many string) string {
	if len(items) == 1 {
		return one
	}
	return many
}
