package dynamic

import (
	"fmt"
	"regexp"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

// hashPrefixedParams maps a command ID to the path parameter whose value is
// displayed with a "#" that is not part of it (order numbers show as "#1001").
var hashPrefixedParams = map[string]string{"order-by-number": "number"}

// dropHashPrefix removes that leading "#" from params in place and returns a
// dry-run hint saying so, or "" when the value had none.
func dropHashPrefix(c registry.Command, params map[string]any) string {
	name, ok := hashPrefixedParams[c.ID]
	if !ok {
		return ""
	}
	v, ok := params[name].(string)
	if !ok || !strings.HasPrefix(strings.TrimSpace(v), "#") {
		return ""
	}
	trimmed := strings.TrimSpace(strings.TrimPrefix(strings.TrimSpace(v), "#"))
	params[name] = trimmed
	return fmt.Sprintf("dropped the leading # from %s: %q is sent as %q", name, v, trimmed)
}

// fillMissingPathParams prompts a human for any {param} in the path template that
// --params left unset, injecting the answers into params so ResolveTemplatedPath
// succeeds. It is the dynamic-leaf counterpart of the shortcut engine's fill:
// the id lives inside the --params JSON (not a flag), so this fills it there.
//
// Non-interactive callers (agents, pipes, CI) are a no-op — a missing param
// stays the structured missing_required_param error, so the fail-fast contract
// is unchanged. --dry-run still prompts: it previews a
// concrete request, which needs the id resolved.
func fillMissingPathParams(_ *cobra.Command, f *cmdutil.Factory, pathTemplate string, params map[string]any) error {
	if !cmdutil.Interactive(f) {
		return nil
	}
	for _, name := range pathParamNames(pathTemplate) {
		if v, ok := params[name]; ok && fmt.Sprint(v) != "" {
			continue
		}
		val, err := interact.Input(name, func(s string) error {
			if strings.TrimSpace(s) == "" {
				return fmt.Errorf("%s is required", name)
			}
			return nil
		})
		if err != nil {
			return err // ErrCanceled on esc/ctrl+c, else a render error
		}
		if val = strings.TrimSpace(val); val != "" {
			params[name] = val
		}
	}
	return nil
}

// pathParamNames returns the {name} segments of a path template, in order.
// Duplicates are kept in order (they resolve to the same params key, so the
// second prompt is skipped once the first fills it).
func pathParamNames(pathTemplate string) []string {
	var names []string
	s := pathTemplate
	for {
		i := strings.Index(s, "{")
		if i < 0 {
			break
		}
		j := strings.Index(s[i:], "}")
		if j < 0 {
			break
		}
		j += i
		names = append(names, s[i+1:j])
		s = s[j+1:]
	}
	return names
}

// placeholderName is the inside of a placeholder: a name, never JSON or a number.
var placeholderName = regexp.MustCompile(`^[A-Za-z][A-Za-z0-9_ .-]*$`)

// isPlaceholder reports whether a --params value is template text left in place
// of a real value, such as {product_id}, {{product_id}} or <id> copied from an example.
func isPlaceholder(s string) bool {
	s = strings.TrimSpace(s)
	var inner string
	switch {
	case strings.HasPrefix(s, "{") && strings.HasSuffix(s, "}"):
		inner = strings.Trim(s, "{}")
	case strings.HasPrefix(s, "<") && strings.HasSuffix(s, ">"):
		inner = strings.Trim(s, "<>")
	default:
		return false
	}
	return placeholderName.MatchString(inner)
}
