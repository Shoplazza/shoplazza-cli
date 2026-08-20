package auth

import (
	"slices"
	"testing"
)

// The table is "flags → reachable steps": which screens the flags leave open,
// not which ones the user's own answers later narrow down. It is the only
// direct assertion on step suppression — at the command seam a skipped screen
// and a closed gate look identical (no prompt, same envelope).
func TestPlan_StepsByFlagCombo(t *testing.T) {
	const store = "demo.myshoplaza.com"
	for _, tc := range []struct {
		name     string
		flags    loginFlags
		gateOpen bool
		want     []loginStep
	}{
		// Gate open: the domains screen is asked unless a flag already answered it.
		{"bare run", loginFlags{}, true, []loginStep{stepDomains}},
		{"store only", loginFlags{storeDomain: store}, true, []loginStep{stepDomains}},
		{"domain given", loginFlags{domain: []string{"products"}}, true, nil},
		{"domain all given", loginFlags{domain: []string{"all"}}, true, nil},
		{"scope given", loginFlags{scope: []string{"read_product"}}, true, nil},
		{"uat given", loginFlags{uat: "uat_x"}, true, nil},
		{"store and domain", loginFlags{storeDomain: store, domain: []string{"orders"}}, true, nil},
		{"store and scope", loginFlags{storeDomain: store, scope: []string{"read_order"}}, true, nil},
		{"store and uat", loginFlags{storeDomain: store, uat: "uat_x"}, true, nil},
		{"domain and scope", loginFlags{domain: []string{"shop"}, scope: []string{"read_shop"}}, true, nil},
		{"uat and domain", loginFlags{uat: "uat_x", domain: []string{"shop"}}, true, nil},
		{"merge-scopes is not a step flag", loginFlags{mergeScopes: true}, true, []loginStep{stepDomains}},

		// Gate closed: never a single screen, whatever the flags say.
		{"gate closed, bare run", loginFlags{}, false, nil},
		{"gate closed, store only", loginFlags{storeDomain: store}, false, nil},
		{"gate closed, domain given", loginFlags{domain: []string{"products"}}, false, nil},
		{"gate closed, uat given", loginFlags{uat: "uat_x"}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan(tc.flags, tc.gateOpen); !slices.Equal(got, tc.want) {
				t.Errorf("plan(%+v, gateOpen=%v) = %v, want %v", tc.flags, tc.gateOpen, got, tc.want)
			}
		})
	}
}

// plan must not mutate its input: the wizard writes answers back into the flag
// variables afterwards and re-reads them.
func TestPlan_DoesNotMutateFlags(t *testing.T) {
	fl := loginFlags{storeDomain: "demo.myshoplaza.com", domain: []string{"products"}}
	_ = plan(fl, true)
	if fl.storeDomain != "demo.myshoplaza.com" || len(fl.domain) != 1 || fl.domain[0] != "products" {
		t.Errorf("plan mutated its input: %+v", fl)
	}
}
