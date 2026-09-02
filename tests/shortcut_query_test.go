// shortcut_query_test.go guards every shortcut's planned QUERY KEYS against the
// embedded spec, the companion to the path guard in shortcut_paths_test.go.
//
// The API drops query params it does not recognise instead of rejecting them,
// so a shortcut that plans a misspelled key returns 200 with an UNFILTERED
// result set — a wrong answer indistinguishable from a right one. That is how
// `orders +search --keyword` shipped sending "query" (the spec param is
// "keyword") and quietly returned every order in the store.

package tests_test

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestShortcutPlanQueryKeysMatchSpec(t *testing.T) {
	// LoadSpec would otherwise pick up a real downloaded cache on dev machines;
	// this test must validate against the embedded spec.
	testenv.IsolateConfigDir(t)
	spec := registry.LoadSpec()
	if spec == nil || len(spec.Modules) == 0 {
		t.Skip("embedded spec is empty; nothing to validate against")
	}

	var checked int
	for _, sc := range allGuardedShortcuts() {
		t.Run(sc.Service+"."+sc.Command, func(t *testing.T) {
			if sc.Plan == nil {
				t.Logf("%s: Execute-only, skipped", sc.Command)
				return
			}
			plan, err := sc.Plan(common.PlanInput{
				Args:  []string{"DUMMY", "DUMMY", "DUMMY", "DUMMY"},
				Flags: newQueryStub(sc),
				Tool:  strings.TrimPrefix(sc.Command, "+"),
			})
			if err != nil {
				t.Skipf("plan rejected stub input: %v", err)
			}
			if len(plan.Query) == 0 {
				return
			}
			cmd, ok := specFindEndpoint(spec, plan.Method, plan.Path)
			if !ok {
				t.Skipf("%s %s matches no spec endpoint (covered by the path guard)", plan.Method, plan.Path)
			}
			checked++
			for key := range plan.Query {
				if !endpointHasQueryParam(cmd, key) {
					t.Errorf("query param %q is not documented on %s %s; the server would drop it and return unfiltered results",
						key, plan.Method, cmd.HTTP.Path)
				}
			}
		})
	}
	if checked == 0 {
		t.Fatal("no shortcut produced a checkable query; the guard would silently pass")
	}
}

// specFindEndpoint is specHasEndpoint's sibling, returning the matched command
// so its parameter list can be inspected.
func specFindEndpoint(spec *registry.Spec, method, concrete string) (registry.Command, bool) {
	method = strings.ToUpper(method)
	cSegs := splitPath(concrete)
	for _, m := range spec.Modules {
		for _, cmd := range m.Commands {
			if strings.ToUpper(cmd.HTTP.Method) != method {
				continue
			}
			if templateMatches(splitPath(cmd.HTTP.Path), cSegs) {
				return cmd, true
			}
		}
	}
	return registry.Command{}, false
}

func endpointHasQueryParam(cmd registry.Command, name string) bool {
	for _, p := range cmd.Parameters {
		if p.In == "query" && p.Name == name {
			return true
		}
	}
	return false
}

// queryStub fills every accessor with a usable value so a Plan emits as many
// query keys as it can. Flags that declare Completions get an allowed value —
// otherwise enum validation rejects the stub and the whole shortcut goes
// unchecked. Bools stay false: they gate mutually exclusive options more often
// than they map to a query param.
type queryStub struct {
	enum map[string]string
}

func newQueryStub(sc common.Shortcut) queryStub {
	s := queryStub{enum: map[string]string{}}
	for _, f := range sc.Flags {
		if len(f.Completions) > 0 {
			s.enum[f.Name] = f.Completions[0]
		}
	}
	return s
}

func (s queryStub) GetString(name string) string {
	if v, ok := s.enum[name]; ok {
		return v
	}
	return "x"
}

func (s queryStub) GetStringSlice(name string) []string { return []string{s.GetString(name)} }
func (s queryStub) GetStringArray(name string) []string { return []string{s.GetString(name)} }
func (queryStub) GetInt(string) int                     { return 1 }
func (queryStub) GetFloat(string) float64               { return 1 }
func (queryStub) GetBool(string) bool                   { return false }
func (queryStub) Changed(string) bool                   { return false }
