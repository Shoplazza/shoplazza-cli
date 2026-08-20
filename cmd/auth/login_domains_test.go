package auth

import (
	"reflect"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
)

// TestDomainOptions_NoAllRow pins the picker's contents. An "all" row used to
// lead the list; ctrl+a already does that job, and a row that cannot be
// combined with any other row does not belong in a multi-select.
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

// TestCollapseAll covers the only path by which the wizard can emit the
// sentinel. Getting this wrong either sends 10 domains where the flag form
// would send one word, or claims "all" from a partial selection.
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
	// An empty domain list must not make "nothing selected" look like "everything".
	if got := collapseAll(nil, nil); got != nil {
		t.Errorf("collapseAll(nil, nil) = %v, want nil", got)
	}
}

// TestCollapseAll_MatchesTheFlag is the invariant behind the collapse: the two
// spellings must grant the same scopes, or the shortcut would quietly change
// what the user authorised.
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
