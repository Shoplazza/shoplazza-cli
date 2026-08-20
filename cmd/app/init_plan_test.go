package appcmd

import (
	"slices"
	"testing"
)

// TestPlan_StepsByFlagCombo pins which screens each flag combination leaves
// open. partner carries --partner or the sole partner auto-selected beforehand.
func TestPlan_StepsByFlagCombo(t *testing.T) {
	for _, tc := range []struct {
		name     string
		flags    initFlags
		gateOpen bool
		want     []initStep
	}{
		{"bare run, several partners", initFlags{}, true, []initStep{stepPartner, stepApp, stepName}},
		{"bare run, sole partner auto-selected", initFlags{partner: "p1"}, true, []initStep{stepApp, stepName}},

		{"partner flag only", initFlags{partner: "p1"}, true, []initStep{stepApp, stepName}},

		// --name answers both the app screen and the name screen.
		{"name only, several partners", initFlags{name: "My App"}, true, []initStep{stepPartner}},
		{"name only, sole partner auto-selected", initFlags{name: "My App", partner: "p1"}, true, nil},
		{"name and partner", initFlags{name: "My App", partner: "p2"}, true, nil},

		// --client-id is complete on its own: the partner is derived from the app.
		{"client-id only", initFlags{clientID: "cid_x"}, true, nil},
		{"client-id and partner", initFlags{clientID: "cid_x", partner: "p1"}, true, nil},

		// Gate closed: no screen, whatever the flags say.
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

// TestPlan_DoesNotMutateFlags pins that plan leaves its input untouched.
func TestPlan_DoesNotMutateFlags(t *testing.T) {
	fl := initFlags{name: "My App", partner: "p1"}
	_ = plan(fl, true)
	if fl != (initFlags{name: "My App", partner: "p1"}) {
		t.Errorf("plan mutated its input: %+v", fl)
	}
}
