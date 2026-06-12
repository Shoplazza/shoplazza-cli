package registry

import (
	"sort"
	"strings"
)

// MaxSchemaDepth caps recursive expansion of object schemas.
const MaxSchemaDepth = 5

// Schema-view selectors recognized by ResolveSpecSchema for leaf commands.
const (
	ViewAll      = "all"
	ViewRequest  = "request"
	ViewResponse = "response"
)

// ResolveSpecSchema interprets a `schema <path>` query and returns a
// JSON-friendly payload. Path forms: "" → modules; "<module>" → overview;
// "<module>.<cmd>[.<sub>]" → leaf detail; "scopes <module>.<cmd>" → scopes.
// view filters leaf payload (all/request/response); ignored on non-leaves.
// found=false means the path didn't resolve.
func ResolveSpecSchema(spec *Spec, path, view string) (any, bool, error) {
	if spec == nil {
		return nil, false, nil
	}

	if rest := strings.TrimPrefix(path, "scopes "); rest != path {
		return resolveScopes(spec, strings.TrimSpace(rest))
	}

	if path == "" {
		return resolveModuleList(spec), true, nil
	}

	// module or module.cmd[.sub]
	parts := strings.Split(path, ".")
	mod, ok := findModule(spec, parts[0])
	if !ok {
		return nil, false, nil
	}
	if len(parts) == 1 {
		return resolveModuleDetail(spec, mod), true, nil
	}

	subPath := parts[1:]
	if cmd, ok := findCommandByPath(mod, subPath); ok {
		return resolveCommandDetail(spec, mod, cmd, view), true, nil
	}
	// Implicit group node (e.g. "products.images" prefixing several leaves).
	if group, ok := resolveImplicitGroup(mod, subPath); ok {
		return group, true, nil
	}
	return nil, false, nil
}

func resolveImplicitGroup(mod Module, prefix []string) (any, bool) {
	cmds := make([]map[string]any, 0)
	for _, c := range mod.Commands {
		if !startsWith(c.Path, prefix) {
			continue
		}
		cmds = append(cmds, map[string]any{
			"command_path": c.Path,
			"http": map[string]any{
				"method": c.HTTP.Method,
				"path":   c.HTTP.Path,
			},
			"summary": c.Summary,
		})
	}
	if len(cmds) == 0 {
		return nil, false
	}
	return map[string]any{
		"module":   mod.Name,
		"commands": cmds,
	}, true
}

func startsWith(path, prefix []string) bool {
	if len(path) <= len(prefix) {
		return false
	}
	for i, seg := range prefix {
		if path[i] != seg {
			return false
		}
	}
	return true
}

func resolveModuleList(spec *Spec) any {
	names := make([]string, 0, len(spec.Modules))
	for _, m := range spec.Modules {
		names = append(names, m.Name)
	}
	sort.Strings(names)
	return map[string]any{"modules": names}
}

func resolveModuleDetail(spec *Spec, mod Module) any {
	cmds := make([]map[string]any, 0, len(mod.Commands))
	for _, c := range mod.Commands {
		cmds = append(cmds, map[string]any{
			"command_path": c.Path,
			"http": map[string]any{
				"method": c.HTTP.Method,
				"path":   c.HTTP.Path,
			},
			"summary": c.Summary,
		})
	}
	return map[string]any{
		"module":   mod.Name,
		"commands": cmds,
	}
}

func resolveCommandDetail(spec *Spec, mod Module, cmd Command, view string) any {
	payload := map[string]any{
		"module":       mod.Name,
		"command_path": cmd.Path,
		"id":           cmd.ID,
		"summary":      cmd.Summary,
		"description":  cmd.Description,
		"http": map[string]any{
			"method": cmd.HTTP.Method,
			"path":   cmd.HTTP.Path,
		},
	}

	includeRequest := view != ViewResponse
	includeResponse := view != ViewRequest

	if includeRequest {
		if len(cmd.Parameters) > 0 {
			payload["parameters"] = cmd.Parameters
		}
		if cmd.Body != nil && len(cmd.Body.Fields) > 0 {
			payload["body"] = expandBody(spec, cmd.Body)
		}
	}
	if includeResponse && cmd.ResponseSchema != "" {
		payload["response"] = expandSchemaRef(spec, cmd.ResponseSchema)
	}
	return payload
}

func resolveScopes(spec *Spec, target string) (any, bool, error) {
	// Stub: per-command scopes not yet modeled in this spec version.
	return map[string]any{
		"target": target,
		"scopes": []any{},
		"note":   "scope metadata not yet available in this spec version",
	}, true, nil
}

func findModule(spec *Spec, name string) (Module, bool) {
	// Fast path: O(1) index built by LoadSpec.
	if spec.moduleIndex != nil {
		idx, ok := spec.moduleIndex[name]
		if !ok {
			return Module{}, false
		}
		return spec.Modules[idx], true
	}
	// Fallback for Spec values not created via LoadSpec (e.g. unit-test fixtures).
	for _, m := range spec.Modules {
		if m.Name == name {
			return m, true
		}
	}
	return Module{}, false
}

func findCommandByPath(mod Module, path []string) (Command, bool) {
	for _, c := range mod.Commands {
		if equalPath(c.Path, path) {
			return c, true
		}
	}
	return Command{}, false
}

func equalPath(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func expandBody(spec *Spec, body *Body) any {
	out := map[string]any{}
	if body.Required {
		out["required"] = true
	}
	out["fields"] = expandFields(spec, body.Fields, map[string]bool{}, 0)
	return out
}

func expandFields(spec *Spec, fields []Field, seen map[string]bool, depth int) []any {
	result := make([]any, 0, len(fields))
	for _, f := range fields {
		item := map[string]any{
			"name": f.Name,
			"type": f.Type,
		}
		if f.Required {
			item["required"] = true
		}
		if f.Description != "" {
			item["description"] = f.Description
		}
		if len(f.Enum) > 0 {
			item["enum"] = f.Enum
		}
		if f.Schema != "" {
			item["schema"] = expandSchemaRefDepth(spec, f.Schema, seen, depth+1)
		}
		if f.Items != nil {
			sub := []Field{*f.Items}
			item["items"] = expandFields(spec, sub, seen, depth+1)[0]
		}
		result = append(result, item)
	}
	return result
}

func expandSchemaRef(spec *Spec, ref string) any {
	return expandSchemaRefDepth(spec, ref, map[string]bool{}, 0)
}

func expandSchemaRefDepth(spec *Spec, ref string, seen map[string]bool, depth int) any {
	if depth >= MaxSchemaDepth {
		return map[string]any{"name": ref, "truncated": true}
	}
	if seen[ref] {
		return map[string]any{"name": ref, "cycle": true}
	}
	sch, ok := spec.Schemas[ref]
	if !ok {
		return map[string]any{"name": ref, "unresolved": true}
	}
	seen[ref] = true
	defer delete(seen, ref)
	return map[string]any{
		"fields": expandFields(spec, sch.Fields, seen, depth),
	}
}
