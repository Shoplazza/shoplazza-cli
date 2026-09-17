package auth

import (
	"reflect"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
)

// TestDomainOptions_NoAllRow pins the picker to the concrete domains, with no
// "all" row: ctrl+a is huh's own select-all.
func TestDomainOptions_NoAllRow(t *testing.T) {
	domains := internalauth.TopLevelDomains()
	opts := domainOptions(domains)
	if len(opts) != len(domains) {
		t.Fatalf("picker has %d rows for %d domains", len(opts), len(domains))
	}
	for i, o := range opts {
		if o.Value != domains[i] {
			t.Errorf("row %d = %q, want %q", i, o.Value, domains[i])
		}
		if o.Value == internalauth.DomainAll {
			t.Errorf("row %d offers the %q sentinel; ctrl+a covers it", i, internalauth.DomainAll)
		}
	}
}

// TestCollapseAll pins when a selection collapses to the "all" sentinel.
func TestCollapseAll(t *testing.T) {
	domains := []string{"a", "b", "c"}
	for _, c := range []struct {
		name     string
		selected []string
		want     []string
	}{
		{"every domain ticked collapses", []string{"a", "b", "c"}, []string{internalauth.DomainAll}},
		{"partial stays explicit", []string{"a", "c"}, []string{"a", "c"}},
		{"one stays explicit", []string{"b"}, []string{"b"}},
		{"none stays empty", nil, nil},
	} {
		t.Run(c.name, func(t *testing.T) {
			if got := collapseAll(c.selected, domains); !reflect.DeepEqual(got, c.want) {
				t.Errorf("collapseAll(%v) = %v, want %v", c.selected, got, c.want)
			}
		})
	}
	// An empty domain list must not make "nothing selected" mean "everything".
	if got := collapseAll(nil, nil); got != nil {
		t.Errorf("collapseAll(nil, nil) = %v, want nil", got)
	}
}

// TestCollapseAll_MatchesTheFlag pins that --domain all and every domain ticked
// grant the same scopes.
func TestCollapseAll_MatchesTheFlag(t *testing.T) {
	domains := internalauth.TopLevelDomains()
	viaFlag, err := internalauth.ExpandDomains([]string{internalauth.DomainAll})
	if err != nil {
		t.Fatal(err)
	}
	viaPicker, err := internalauth.ExpandDomains(domains)
	if err != nil {
		t.Fatal(err)
	}
	set := func(in []string) map[string]bool {
		m := map[string]bool{}
		for _, s := range in {
			m[s] = true
		}
		return m
	}
	if !reflect.DeepEqual(set(viaFlag), set(viaPicker)) {
		t.Errorf("--domain all grants %v; ticking every domain grants %v", viaFlag, viaPicker)
	}
}
