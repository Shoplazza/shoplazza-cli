package appcmd

import (
	"slices"
	"testing"
)

// TestStepsFor_StepsByFlagCombo pins which screens each flag combination leaves
// open. partner carries --partner or the sole partner auto-selected beforehand.
func TestStepsFor_StepsByFlagCombo(t *testing.T) {
	for _, tc := range []struct {
		name  string
		flags initFlags
		want  []initStep
	}{
		{"bare run, several partners", initFlags{}, []initStep{stepPartner, stepApp, stepName}},
		{"bare run, sole partner auto-selected", initFlags{partner: "p1"}, []initStep{stepApp, stepName}},

		{"partner flag only", initFlags{partner: "p1"}, []initStep{stepApp, stepName}},

		// --name answers both the app screen and the name screen.
		{"name only, several partners", initFlags{name: "My App"}, []initStep{stepPartner}},
		{"name only, sole partner auto-selected", initFlags{name: "My App", partner: "p1"}, nil},
		{"name and partner", initFlags{name: "My App", partner: "p2"}, nil},

		// --client-id is complete on its own: the partner is derived from the app.
		{"client-id only", initFlags{clientID: "cid_x"}, nil},
		{"client-id and partner", initFlags{clientID: "cid_x", partner: "p1"}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stepsFor(tc.flags); !slices.Equal(got, tc.want) {
				t.Errorf("stepsFor(%+v) = %v, want %v", tc.flags, got, tc.want)
			}
		})
	}
}

// TestStepsFor_DoesNotMutateFlags pins that stepsFor leaves its input untouched.
func TestStepsFor_DoesNotMutateFlags(t *testing.T) {
	fl := initFlags{name: "My App", partner: "p1"}
	_ = stepsFor(fl)
	if fl != (initFlags{name: "My App", partner: "p1"}) {
		t.Errorf("stepsFor mutated its input: %+v", fl)
	}
}
