package auth

import (
	"strings"
	"testing"

	internalauth "github.com/Shoplazza/shoplazza-cli/v2/internal/auth"
)

// TestLoginSummaryRows_ExpandsAll pins that the card spells out the domains the
// "all" sentinel granted, wrapped to the card width.
func TestLoginSummaryRows_ExpandsAll(t *testing.T) {
	rows := loginSummaryRows("demo.myshoplaza.com", []string{internalauth.DomainAll}, nil)
	if len(rows) != 2 {
		t.Fatalf("rows = %q, want a store row and a domains row", rows)
	}
	if !strings.Contains(rows[0], "demo.myshoplaza.com") {
		t.Errorf("store row = %q", rows[0])
	}
	domainsRow := rows[1]
	for _, d := range internalauth.TopLevelDomains() {
		if !strings.Contains(domainsRow, d) {
			t.Errorf("domains row is missing %q: %q", d, domainsRow)
		}
	}
	if strings.Contains(domainsRow, internalauth.DomainAll+",") {
		t.Errorf("domains row still shows the sentinel: %q", domainsRow)
	}
	if !strings.Contains(domainsRow, "\n") {
		t.Errorf("ten domains must wrap onto more than one line: %q", domainsRow)
	}
	for _, line := range strings.Split(domainsRow, "\n") {
		if len(line) > labelColumn+summaryListWidth {
			t.Errorf("line exceeds the card width: %q", line)
		}
	}
}

// TestLoginSummaryRows_AccountOnlyAndScopes pins that a store-less login reads
// as account-only and that --scope is summarised as scopes, not domains.
func TestLoginSummaryRows_AccountOnlyAndScopes(t *testing.T) {
	rows := loginSummaryRows("", nil, []string{"read_product", "read_order"})
	if !strings.Contains(rows[0], "(account only)") {
		t.Errorf("store row = %q, want the account-only marker", rows[0])
	}
	if !strings.Contains(rows[1], "Scopes") || !strings.Contains(rows[1], "read_order") {
		t.Errorf("scope row = %q", rows[1])
	}
}

// labelColumn is the card's label column width plus its separating space; only
// the value column is wrapped to summaryListWidth.
const labelColumn = 11
