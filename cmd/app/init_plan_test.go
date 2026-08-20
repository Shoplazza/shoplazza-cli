package appcmd

import (
	"slices"
	"testing"
)

// The table is "flags → reachable steps": which screens the flags leave open,
// not which ones the user's own answers later narrow down (picking an existing
// app closes the name screen — that is the form's live hide func, not plan's
// job). It is the only direct assertion on step suppression: at the command seam
// a skipped screen and a closed gate look identical.
//
// partner carries either --partner or the sole partner auto-selected before
// plan runs, so a single-partner account is spelled the same way as an explicit
// flag — that is exactly the point of doing the auto-select first.
func TestPlan_StepsByFlagCombo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flags    initFlags
		gateOpen bool
		want     []initStep
	}{
		// Bare run: everything is open. Multiple partners means the partner
		// screen leads; a single partner was already resolved for us.
		{"bare run, several partners", initFlags{}, true, []initStep{stepPartner, stepApp, stepName}},
		{"bare run, sole partner auto-selected", initFlags{partner: "p1"}, true, []initStep{stepApp, stepName}},

		// --partner alone answers screen 1 only.
		{"partner flag only", initFlags{partner: "p1"}, true, []initStep{stepApp, stepName}},

		// --name answers BOTH the app screen (it says "create a new one") and
		// the name screen, so at most the partner is left to ask.
		{"name only, several partners", initFlags{name: "My App"}, true, []initStep{stepPartner}},
		{"name only, sole partner auto-selected", initFlags{name: "My App", partner: "p1"}, true, nil},
		{"name and partner", initFlags{name: "My App", partner: "p2"}, true, nil},

		// --client-id is complete on its own: the owning partner is derived from
		// the app, so not even screen 1 applies.
		{"client-id only", initFlags{clientID: "cid_x"}, true, nil},
		{"client-id and partner", initFlags{clientID: "cid_x", partner: "p1"}, true, nil},

		// Gate closed: never a single screen, whatever the flags say.
		{"gate closed, bare run", initFlags{}, false, nil},
		{"gate closed, partner flag", initFlags{partner: "p1"}, false, nil},
		{"gate closed, name only", initFlags{name: "My App"}, false, nil},
		{"gate closed, name and partner", initFlags{name: "My App", partner: "p1"}, false, nil},
		{"gate closed, client-id", initFlags{clientID: "cid_x"}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan(tc.flags, tc.gateOpen); !slices.Equal(got, tc.want) {
				t.Errorf("plan(%+v, gateOpen=%v) = %v, want %v", tc.flags, tc.gateOpen, got, tc.want)
			}
		})
	}
}

// plan must not mutate its input: the wizard writes the answers back into the
// same struct afterwards and the caller re-reads them.
func TestPlan_DoesNotMutateFlags(t *testing.T) {
	fl := initFlags{name: "My App", partner: "p1"}
	_ = plan(fl, true)
	if fl != (initFlags{name: "My App", partner: "p1"}) {
		t.Errorf("plan mutated its input: %+v", fl)
	}
}
