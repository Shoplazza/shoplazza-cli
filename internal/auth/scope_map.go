package auth

import (
	_ "embed"
	"encoding/json"
	"errors"
	"fmt"
	"sort"
	"strings"
)

//go:embed scope_map.json
var scopeMapJSON []byte

// scopeEntry mirrors the JSON shape per module. Scope is a pointer so JSON
// `null` is distinguishable from a missing field (the former is "no scope
// required"; the latter would be a malformed leaf and rejected at init).
type scopeEntry struct {
	Scope    *string               `json:"scope,omitempty"`
	Children map[string]scopeEntry `json:"children,omitempty"`
}

// moduleScopeBase: flat lookup keyed by dotted module path, value is the
// permission domain base (empty string = "no scope required"). Aggregate
// parents are intentionally absent — only their sub-paths appear here.
// A corrupt scope_map.json panics at init: security-relevant table.
var moduleScopeBase map[string]string

func init() {
	var raw map[string]scopeEntry
	if err := json.Unmarshal(scopeMapJSON, &raw); err != nil {
		panic("auth: corrupt scope_map.json: " + err.Error())
	}
	moduleScopeBase = make(map[string]string, len(raw))
	for key, entry := range raw {
		if err := flattenScopeEntry(key, entry, moduleScopeBase); err != nil {
			panic("auth: scope_map.json: " + err.Error())
		}
	}
}

func flattenScopeEntry(path string, entry scopeEntry, out map[string]string) error {
	hasChildren := len(entry.Children) > 0
	if hasChildren {
		for child, childEntry := range entry.Children {
			if err := flattenScopeEntry(path+"."+child, childEntry, out); err != nil {
				return err
			}
		}
		// Add a parent leaf only when scope is explicitly given (a string,
		// including ""). With *string, JSON null is indistinguishable from an
		// absent key, so both mean "no parent leaf" when children are present.
		if entry.Scope != nil {
			out[path] = *entry.Scope
		}
		return nil
	}
	if entry.Scope != nil {
		out[path] = *entry.Scope
	} else {
		out[path] = ""
	}
	return nil
}

// ScopeBase returns the permission domain base for a module path.
// ok=true with base=="" means "known, no scope required". ok=false is unknown.
// Lookup is longest-prefix on dotted components.
func ScopeBase(module string) (string, bool) {
	if module == "" {
		return "", false
	}
	key := module
	for {
		if base, found := moduleScopeBase[key]; found {
			return base, true
		}
		i := strings.LastIndex(key, ".")
		if i < 0 {
			return "", false
		}
		key = key[:i]
	}
}

// ReadScope returns "read_<base>". ok=false when module is unknown or no-scope.
func ReadScope(module string) (string, bool) {
	base, ok := ScopeBase(module)
	if !ok || base == "" {
		return "", false
	}
	return "read_" + base, true
}

// WriteScope returns "write_<base>". ok=false when module is unknown or no-scope.
func WriteScope(module string) (string, bool) {
	base, ok := ScopeBase(module)
	if !ok || base == "" {
		return "", false
	}
	return "write_" + base, true
}

// ModuleScopes returns [read,write] for a module, or nil if unknown / no-scope.
func ModuleScopes(module string) []string {
	r, rok := ReadScope(module)
	w, wok := WriteScope(module)
	if !rok && !wok {
		return nil
	}
	return []string{r, w}
}

// DomainAll is the sentinel --domain value that grants every known scope.
const DomainAll = "all"

// domainImpliedReadScopes maps a domain to extra READ-only scopes its CLI
// workflows need beyond the domain's own module scope. Example: `themes serve`
// and `themes share` call GET /shop to build the preview-URL banner, so --domain
// themes must also grant read_shop (read-only — never write_shop).
var domainImpliedReadScopes = map[string][]string{
	"themes": {"read_shop"},
}

// ExpandDomain resolves a --domain value passed at login.
//   - "all"                       → every scope across every module that needs one.
//   - parent w/ children & scope  → union of parent scope + child scopes.
//   - parent w/ children only     → union of child scopes.
//   - leaf (e.g. "discounts")     → [read_<base>, write_<base>].
//   - (nil, nil)                  → known module that contributes nothing.
//   - (nil, err)                  → unknown name.
func ExpandDomain(domain string) ([]string, error) {
	if domain == "" {
		return nil, errors.New("domain must not be empty")
	}
	if domain == DomainAll {
		return allScopes(), nil
	}

	seen := map[string]struct{}{}
	var out []string
	add := func(base string) {
		if base == "" {
			return
		}
		for _, s := range []string{"read_" + base, "write_" + base} {
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}

	directBase, directOK := moduleScopeBase[domain]
	if directOK {
		add(directBase)
	}

	prefix := domain + "."
	childMatched := false
	for key, base := range moduleScopeBase {
		if !strings.HasPrefix(key, prefix) {
			continue
		}
		childMatched = true
		add(base)
	}

	if !directOK && !childMatched {
		return nil, fmt.Errorf("unknown domain: %s", domain)
	}
	// Layer in any read-only scopes this domain's workflows imply (e.g. themes
	// → read_shop). Added directly, NOT via add(), so we grant only the read
	// scope without the paired write_ scope.
	for _, s := range domainImpliedReadScopes[domain] {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	if len(out) == 0 {
		return nil, nil
	}
	sort.Strings(out)
	return out, nil
}

// SupportedDomains returns every name ExpandDomain accepts (top-level +
// aggregate parents + dotted sub-paths), sorted.
func SupportedDomains() []string {
	seen := map[string]struct{}{}
	for key := range moduleScopeBase {
		top := key
		if i := strings.Index(key, "."); i >= 0 {
			top = key[:i]
		}
		seen[top] = struct{}{}
		if top != key {
			seen[key] = struct{}{} // also expose dotted sub-paths
		}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// TopLevelDomains returns only top-level domain names (no dotted sub-paths),
// sorted. Used by help text where the dotted list would be too noisy.
func TopLevelDomains() []string {
	seen := map[string]struct{}{}
	for key := range moduleScopeBase {
		top := key
		if i := strings.Index(key, "."); i >= 0 {
			top = key[:i]
		}
		seen[top] = struct{}{}
	}
	out := make([]string, 0, len(seen))
	for k := range seen {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

// ExpandDomains returns the deduped union of scopes for each domain, preserving
// first-occurrence order. Surfaces the first unknown domain as an error.
func ExpandDomains(domains []string) ([]string, error) {
	if len(domains) == 0 {
		return nil, nil
	}
	seen := map[string]struct{}{}
	var out []string
	for _, d := range domains {
		scopes, err := ExpandDomain(d)
		if err != nil {
			return nil, err
		}
		for _, s := range scopes {
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	return out, nil
}

// DedupePreserveOrder drops duplicates, keeping each value's first occurrence.
func DedupePreserveOrder(in []string) []string {
	if len(in) == 0 {
		return nil
	}
	seen := map[string]struct{}{}
	out := make([]string, 0, len(in))
	for _, s := range in {
		if _, dup := seen[s]; dup {
			continue
		}
		seen[s] = struct{}{}
		out = append(out, s)
	}
	return out
}

func allScopes() []string {
	seen := map[string]struct{}{}
	var out []string
	for _, base := range moduleScopeBase {
		if base == "" {
			continue
		}
		for _, s := range []string{"read_" + base, "write_" + base} {
			if _, dup := seen[s]; dup {
				continue
			}
			seen[s] = struct{}{}
			out = append(out, s)
		}
	}
	sort.Strings(out)
	return out
}
