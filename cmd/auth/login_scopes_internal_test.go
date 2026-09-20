package auth

import (
	"strings"
	"testing"
)

func containsAll(got []string, want ...string) bool {
	set := make(map[string]bool, len(got))
	for _, s := range got {
		set[s] = true
	}
	for _, w := range want {
		if !set[w] {
			return false
		}
	}
	return true
}

// TestDomainFlagHelp_ListsApp pins that the --domain help lists app, generated
// from the scope map.
func TestDomainFlagHelp_ListsApp(t *testing.T) {
	_, list, ok := strings.Cut(domainFlagHelp(), "Available: ")
	if !ok {
		t.Fatalf("--domain help has no available-domain list: %s", domainFlagHelp())
	}
	domains := strings.Split(strings.TrimSuffix(strings.TrimSpace(list), "."), ", ")
	if !containsAll(domains, "app") {
		t.Fatalf("--domain help does not list app; available = %v", domains)
	}
}

// A narrow re-login must carry the prior grant along: authorization replaces
// the account's granted set server-side, so requesting only read_inventory
// would otherwise revoke every other scope mid-task.
func TestUnionWithGranted_KeepsPriorGrant(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_inventory", "write_inventory"},
		[]string{"read_product", "write_product", "read_inventory"},
	)
	if !containsAll(got, "read_inventory", "write_inventory", "read_product", "write_product") {
		t.Fatalf("union = %v, want request ∪ grant", got)
	}
	if kept != 2 { // read_product + write_product carried over; read_inventory overlaps
		t.Fatalf("kept = %d, want 2", kept)
	}
	if got[0] != "read_inventory" || got[1] != "write_inventory" {
		t.Fatalf("union = %v, want the request's scopes first", got)
	}
}

func TestUnionWithGranted_NoPriorGrant(t *testing.T) {
	got, kept := unionWithGranted([]string{"read_shop"}, nil)
	if len(got) != 1 || got[0] != "read_shop" || kept != 0 {
		t.Fatalf("union = %v kept=%d, want unchanged request", got, kept)
	}
}

func TestUnionWithGranted_RequestSupersetOfGrant(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_product", "write_product"},
		[]string{"read_product"},
	)
	if len(got) != 2 || kept != 0 {
		t.Fatalf("union = %v kept=%d, want no carry-over when request covers grant", got, kept)
	}
}

func TestUnionWithGranted_DedupesRequest(t *testing.T) {
	got, kept := unionWithGranted(
		[]string{"read_shop", "read_shop"},
		[]string{"read_shop"},
	)
	if len(got) != 1 || kept != 0 {
		t.Fatalf("union = %v kept=%d, want dedup without counting overlaps as kept", got, kept)
	}
}
