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
		// Gate open, no flags: both screens, store first.
		{"bare run", loginFlags{}, true, []loginStep{stepStore, stepDomains}},
		{"merge-scopes is not a step flag", loginFlags{mergeScopes: true}, true, []loginStep{stepStore, stepDomains}},

		// -s answers the store screen; permission flags answer the domains one.
		{"store only", loginFlags{storeDomain: store}, true, []loginStep{stepDomains}},
		{"domain given", loginFlags{domain: []string{"products"}}, true, []loginStep{stepStore}},
		{"domain all given", loginFlags{domain: []string{"all"}}, true, []loginStep{stepStore}},
		{"scope given", loginFlags{scope: []string{"read_product"}}, true, []loginStep{stepStore}},
		{"domain and scope", loginFlags{domain: []string{"shop"}, scope: []string{"read_shop"}}, true, []loginStep{stepStore}},
		{"domain given, merge-scopes", loginFlags{domain: []string{"shop"}, mergeScopes: true}, true, []loginStep{stepStore}},

		// Both answered: zero interaction.
		{"store and domain", loginFlags{storeDomain: store, domain: []string{"orders"}}, true, nil},
		{"store and domain all", loginFlags{storeDomain: store, domain: []string{"all"}}, true, nil},
		{"store and scope", loginFlags{storeDomain: store, scope: []string{"read_order"}}, true, nil},
		{"store, domain and scope", loginFlags{storeDomain: store, domain: []string{"shop"}, scope: []string{"read_shop"}}, true, nil},
		{"store and domain, merge-scopes", loginFlags{storeDomain: store, domain: []string{"shop"}, mergeScopes: true}, true, nil},

		// --uat is the CI fast path: no screen, whatever else is given.
		{"uat given", loginFlags{uat: "uat_x"}, true, nil},
		{"uat and store", loginFlags{uat: "uat_x", storeDomain: store}, true, nil},
		{"uat and domain", loginFlags{uat: "uat_x", domain: []string{"shop"}}, true, nil},
		{"uat and scope", loginFlags{uat: "uat_x", scope: []string{"read_shop"}}, true, nil},
		{"uat, store and domain", loginFlags{uat: "uat_x", storeDomain: store, domain: []string{"shop"}}, true, nil},

		// Gate closed: never a single screen, whatever the flags say.
		{"gate closed, bare run", loginFlags{}, false, nil},
		{"gate closed, store only", loginFlags{storeDomain: store}, false, nil},
		{"gate closed, domain given", loginFlags{domain: []string{"products"}}, false, nil},
		{"gate closed, scope given", loginFlags{scope: []string{"read_product"}}, false, nil},
		{"gate closed, store and domain", loginFlags{storeDomain: store, domain: []string{"orders"}}, false, nil},
		{"gate closed, uat given", loginFlags{uat: "uat_x"}, false, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			if got := plan(tc.flags, tc.gateOpen); !slices.Equal(got, tc.want) {
				t.Errorf("plan(%+v, gateOpen=%v) = %v, want %v", tc.flags, tc.gateOpen, got, tc.want)
			}
		})
	}
}

// Order is part of the contract, not incidental: the wizard builds its groups
// in this order, and huh walks groups in the order they were built. The store
// is asked before the permissions that store will be reached with.
func TestPlan_StoreIsAskedBeforeDomains(t *testing.T) {
	steps := plan(loginFlags{}, true)
	i, j := slices.Index(steps, stepStore), slices.Index(steps, stepDomains)
	if i < 0 || j < 0 || i > j {
		t.Fatalf("plan = %v, want the store screen before the domains screen", steps)
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
