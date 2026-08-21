package auth

import (
	"slices"
	"testing"
)

// TestStepsFor_StepsByFlagCombo pins which screens each flag combination leaves open.
func TestStepsFor_StepsByFlagCombo(t *testing.T) {
	const store = "demo.myshoplaza.com"
	for _, tc := range []struct {
		name  string
		flags loginFlags
		want  []loginStep
	}{
		{"bare run", loginFlags{}, []loginStep{stepStore, stepDomains}},
		{"merge-scopes is not a step flag", loginFlags{mergeScopes: true}, []loginStep{stepStore, stepDomains}},

		// -s answers the store screen; --domain / --scope answer the domains one.
		{"store only", loginFlags{storeDomain: store}, []loginStep{stepDomains}},
		{"domain given", loginFlags{domain: []string{"products"}}, []loginStep{stepStore}},
		{"domain all given", loginFlags{domain: []string{"all"}}, []loginStep{stepStore}},
		{"scope given", loginFlags{scope: []string{"read_product"}}, []loginStep{stepStore}},
		{"domain and scope", loginFlags{domain: []string{"shop"}, scope: []string{"read_shop"}}, []loginStep{stepStore}},
		{"domain given, merge-scopes", loginFlags{domain: []string{"shop"}, mergeScopes: true}, []loginStep{stepStore}},

		{"store and domain", loginFlags{storeDomain: store, domain: []string{"orders"}}, nil},
		{"store and domain all", loginFlags{storeDomain: store, domain: []string{"all"}}, nil},
		{"store and scope", loginFlags{storeDomain: store, scope: []string{"read_order"}}, nil},
		{"store, domain and scope", loginFlags{storeDomain: store, domain: []string{"shop"}, scope: []string{"read_shop"}}, nil},
		{"store and domain, merge-scopes", loginFlags{storeDomain: store, domain: []string{"shop"}, mergeScopes: true}, nil},

		// --uat: no screen, whatever else is given.
		{"uat given", loginFlags{uat: "uat_x"}, nil},
		{"uat and store", loginFlags{uat: "uat_x", storeDomain: store}, nil},
		{"uat and domain", loginFlags{uat: "uat_x", domain: []string{"shop"}}, nil},
		{"uat and scope", loginFlags{uat: "uat_x", scope: []string{"read_shop"}}, nil},
		{"uat, store and domain", loginFlags{uat: "uat_x", storeDomain: store, domain: []string{"shop"}}, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := stepsFor(tc.flags); !slices.Equal(got, tc.want) {
				t.Errorf("stepsFor(%+v) = %v, want %v", tc.flags, got, tc.want)
			}
		})
	}
}

// TestStepsFor_StoreIsAskedBeforeDomains pins the step order: the wizard builds
// its groups in this order, and huh walks groups in build order.
func TestStepsFor_StoreIsAskedBeforeDomains(t *testing.T) {
	steps := stepsFor(loginFlags{})
	i, j := slices.Index(steps, stepStore), slices.Index(steps, stepDomains)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("stepsFor = %v, want the store screen before the domains screen", steps)
	}
}

// TestStepsFor_DoesNotMutateFlags pins that stepsFor leaves its input untouched.
func TestStepsFor_DoesNotMutateFlags(t *testing.T) {
	fl := loginFlags{storeDomain: "demo.myshoplaza.com", domain: []string{"products"}}
	_ = stepsFor(fl)
	if fl.storeDomain != "demo.myshoplaza.com" || len(fl.domain) != 1 || fl.domain[0] != "products" {
		t.Errorf("stepsFor mutated its input: %+v", fl)
	}
}
