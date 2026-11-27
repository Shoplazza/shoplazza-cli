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

	"github.com/spf13/cobra"
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

// checkBodyFields rejects --data keys the command's body does not declare, at any
// depth the metadata describes. A top-level key that is a path/query parameter
// gets its own subtype, so a caller knows to move it rather than fix a typo.
func checkBodyFields(c registry.Command, spec *registry.Spec, body any, binder *paramBinder, schemaHint string) error {
	m, _ := body.(map[string]any)
	if c.Body == nil || len(c.Body.Fields) == 0 || spec == nil || len(m) == 0 {
		return nil
	}
	top := map[string]bool{}
	for _, f := range c.Body.Fields {
		top[f.Name] = true
	}
	var misplaced, flags []string
	for _, p := range commandParams(c) {
		if _, given := m[p.Name]; !given || top[p.Name] {
			continue
		}
		misplaced = append(misplaced, p.Name)
		if flag := binder.flagFor(p.Name); flag != "" {
			flags = append(flags, flag)
		}
	}
	if len(misplaced) > 0 {
		hint := "pass " + plural(misplaced, "it", "them") + " in --params"
		if len(flags) == len(misplaced) {
			hint = "pass " + plural(misplaced, "it", "them") + " as " + strings.Join(flags, ", ")
		}
		return output.ErrValidation("%s %s of this command, not %s", strings.Join(misplaced, ", "),
			plural(misplaced, "is a path/query parameter", "are path/query parameters"),
			plural(misplaced, "a request-body field", "request-body fields")).
			WithParam(misplaced[0]).
			WithSubtype(output.SubtypeMisplacedParam).
			WithHint(hint + "; " + schemaHint + " for details")
	}

	var unknown, near []string
	undeclaredFields(spec, c.Body.Fields, m, "", func(path, key string, siblings []string) {
		unknown = append(unknown, path)
		if like := closestName(key, siblings); like != "" {
			near = append(near, path+" -> "+like)
		}
	})
	if len(unknown) == 0 {
		return nil
	}
	hint := schemaHint + " to see the body fields"
	switch {
	case len(unknown) == 1 && len(near) == 1:
		hint = "did you mean " + strings.TrimPrefix(near[0], unknown[0]+" -> ") + "?; " + hint
	case len(near) > 0:
		hint = "did you mean: " + strings.Join(near, ", ") + "; " + hint
	}
	return output.ErrValidation("unknown --data %s: %s", plural(unknown, "field", "fields"), strings.Join(unknown, ", ")).
		WithParam(unknown[0]).
		WithSubtype(output.SubtypeUnknownField).
		WithHint(hint)
}

// undeclaredFields walks value against fields and reports each key no field
// declares. It descends only where the metadata lists a node's fields and the
// value has that shape.
func undeclaredFields(spec *registry.Spec, fields []registry.Field, value map[string]any, prefix string, report func(path, key string, siblings []string)) {
	byName := make(map[string]registry.Field, len(fields))
	names := make([]string, len(fields))
	for i, f := range fields {
		byName[f.Name] = f
		names[i] = f.Name
	}
	for _, key := range slices.Sorted(maps.Keys(value)) {
		path := prefix + key
		f, declared := byName[key]
		if !declared {
			report(path, key, names)
			continue
		}
		nested := nestedFields(spec, f)
		if nested == nil {
			continue
		}
		switch v := value[key].(type) {
		case map[string]any:
			if f.Type == "object" {
				undeclaredFields(spec, nested, v, path+".", report)
			}
		case []any:
			if f.Type != "array" {
				continue
			}
			for i, item := range v {
				if obj, ok := item.(map[string]any); ok {
					undeclaredFields(spec, nested, obj, fmt.Sprintf("%s[%d].", path, i), report)
				}
			}
		}
	}
}

// nestedFields returns the fields the metadata lists under an object or
// array-of-objects field, or nil when its keys are the caller's to choose.
func nestedFields(spec *registry.Spec, f registry.Field) []registry.Field {
	schema := f.Schema
	if f.Type == "array" && f.Items != nil {
		schema = f.Items.Schema
	}
	if schema == "" || strings.HasPrefix(schema, "google.protobuf.") {
		return nil
	}
	return spec.Schemas[schema].Fields
}

// closestName returns the candidate nearest to name by cobra's suggestion rules, or "".
func closestName(name string, candidates []string) string {
	probe := &cobra.Command{SuggestionsMinimumDistance: 2}
	for _, candidate := range candidates {
		probe.AddCommand(&cobra.Command{Use: candidate, Run: func(*cobra.Command, []string) {}})
	}
	if near := probe.SuggestionsFor(name); len(near) > 0 {
		return near[0]
	}
	return ""
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
