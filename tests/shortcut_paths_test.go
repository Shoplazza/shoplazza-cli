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

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
	"github.com/Shoplazza/shoplazza-cli/v2/internal/testenv"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	customershortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/customers"
	discountshortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/discounts"
	ordershortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/orders"
	productshortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/products"
	shopshortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/shop"
	themeshortcuts "github.com/Shoplazza/shoplazza-cli/v2/shortcuts/themes"
)

// allGuardedShortcuts is the shortcut set the spec guards in this package run
// over. Every shortcut package belongs here — one left out is one whose drift
// from the spec nothing catches.
func allGuardedShortcuts() []common.Shortcut {
	var all []common.Shortcut
	all = append(all, productshortcuts.Shortcuts()...)
	all = append(all, discountshortcuts.Shortcuts()...)
	all = append(all, ordershortcuts.Shortcuts()...)
	all = append(all, customershortcuts.Shortcuts()...)
	all = append(all, shopshortcuts.Shortcuts()...)
	all = append(all, themeshortcuts.Shortcuts()...)
	return all
}

func TestShortcutPlanPathsMatchSpec(t *testing.T) {
	// LoadSpec would otherwise pick up a real downloaded cache on dev machines;
	// this test must validate against the embedded spec.
	testenv.IsolateConfigDir(t)
	spec := registry.LoadSpec()
	if spec == nil || len(spec.Modules) == 0 {
		t.Skip("embedded spec is empty; nothing to validate against")
	}

	for _, sc := range allGuardedShortcuts() {
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

// stubFlagSet returns stub values for every flag accessor. GetString is
// non-empty so a flag interpolated into a path segment still matches a
// {placeholder}; shortcuts that validate the value further error out and skip.
type stubFlagSet struct{}

func (stubFlagSet) GetString(string) string        { return "x" }
func (stubFlagSet) GetInt(string) int              { return 0 }
func (stubFlagSet) GetFloat(string) float64        { return 0 }
func (stubFlagSet) GetBool(string) bool            { return false }
func (stubFlagSet) GetStringSlice(string) []string { return nil }
func (stubFlagSet) GetStringArray(string) []string { return nil }
func (stubFlagSet) Changed(string) bool            { return false }
