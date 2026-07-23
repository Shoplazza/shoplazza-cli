// shortcut_paths_test.go guards every shortcut's PlannedRequest path against
// the embedded spec. Drives each Shortcut.Plan with a stub PlanInput, then
// checks that (method, path) matches some spec endpoint template — paths
// with {placeholders} match any segment.
//
// Catches the class of bug where a shortcut's hand-written URL drifts from
// the spec (e.g. a hand-written /discounts/cancels while the spec endpoint
// is /discounts/cancel).
//
// Shortcuts whose Plan needs flag values to pass validation are tolerated:
// any Plan that returns an error under the stub input is logged and skipped.
// Those paths are still exercised by the e2e tests in this package.

package tests_test

import (
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/shortcuts/common"
	discountshortcuts "github.com/Shoplazza/shoplazza-cli/shortcuts/discounts"
	productshortcuts "github.com/Shoplazza/shoplazza-cli/shortcuts/products"
)

func TestShortcutPlanPathsMatchSpec(t *testing.T) {
	spec := registry.LoadSpec()
	if spec == nil || len(spec.Modules) == 0 {
		t.Skip("embedded spec is empty; nothing to validate against")
	}

	all := append([]common.Shortcut{}, discountshortcuts.Shortcuts()...)
	all = append(all, productshortcuts.Shortcuts()...)

	for _, sc := range all {
		sc := sc
		t.Run(sc.Service+"."+sc.Command, func(t *testing.T) {
			if sc.Plan == nil {
				// Execute-only shortcut; no single planned path to verify here.
				t.Logf("%s: Execute-only, skipped", sc.Command)
				return
			}
			in := common.PlanInput{
				// Generous — every Args[i] access up to 4 is covered.
				Args:  []string{"DUMMY", "DUMMY", "DUMMY", "DUMMY"},
				Flags: stubFlagSet{},
				Tool:  strings.TrimPrefix(sc.Command, "+"),
			}
			plan, err := sc.Plan(in)
			if err != nil {
				// Required-flag or value-shape validation may fail under
				// stub flags. The e2e tests cover those code paths with
				// real inputs; this guard only checks path-vs-spec drift.
				t.Skipf("plan rejected stub input: %v", err)
			}
			if !specHasEndpoint(spec, plan.Method, plan.Path) {
				t.Errorf("%s %s does not match any spec endpoint template", plan.Method, plan.Path)
			}
		})
	}
}

// specHasEndpoint returns true when some spec command has the same method
// and a path template that matches the concrete path segment-by-segment,
// treating "{name}" segments as wildcards.
func specHasEndpoint(spec *registry.Spec, method, concrete string) bool {
	method = strings.ToUpper(method)
	cSegs := splitPath(concrete)
	for _, m := range spec.Modules {
		for _, cmd := range m.Commands {
			if strings.ToUpper(cmd.HTTP.Method) != method {
				continue
			}
			if templateMatches(splitPath(cmd.HTTP.Path), cSegs) {
				return true
			}
		}
	}
	return false
}

func splitPath(p string) []string {
	p = strings.Trim(p, "/")
	if p == "" {
		return nil
	}
	return strings.Split(p, "/")
}

func templateMatches(template, concrete []string) bool {
	if len(template) != len(concrete) {
		return false
	}
	for i, seg := range template {
		if strings.HasPrefix(seg, "{") && strings.HasSuffix(seg, "}") {
			continue
		}
		if seg != concrete[i] {
			return false
		}
	}
	return true
}

// stubFlagSet returns stub values for every flag accessor. GetString returns a
// non-empty placeholder so that shortcuts which interpolate flag values into URL
// path segments (e.g. --id flags) produce a non-empty segment that the
// templateMatches wildcard logic can accept. Shortcuts that impose additional
// validation on the flag value will return a Plan error and be skipped.
type stubFlagSet struct{}

func (stubFlagSet) GetString(string) string        { return "x" }
func (stubFlagSet) GetInt(string) int              { return 0 }
func (stubFlagSet) GetFloat(string) float64        { return 0 }
func (stubFlagSet) GetBool(string) bool            { return false }
func (stubFlagSet) GetStringSlice(string) []string { return nil }
func (stubFlagSet) Changed(string) bool            { return false }
