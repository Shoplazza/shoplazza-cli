package dynamic

import (
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/cmdutil"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/interact"
)

// fillMissingPathParams prompts a human for any {param} in the path template that
// --params left unset, injecting the answers into params so ResolveTemplatedPath
// succeeds. It is the dynamic-leaf counterpart of the shortcut engine's fill:
// the id lives inside the --params JSON (not a flag), so this fills it there.
//
// Non-interactive callers (agents, pipes, CI) are a no-op — a missing param
// stays the structured "missing required path parameter" error, so the
// fail-fast contract is unchanged. --dry-run still prompts: it previews a
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
