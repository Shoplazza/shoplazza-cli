package auth

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/registry"
)

func TestScopeBase_KnownModules(t *testing.T) {
	cases := []struct {
		module string
		base   string
		ok     bool
	}{
		{"products", "product", true},
		{"discounts", "price_rules", true},
		{"webhook", "", true}, // known, but no scope required
		{"shop.blogs", "collection", true},
		{"products.locations", "product", true},
		{"shop.files", "", true},
	}
	for _, c := range cases {
		t.Run(c.module, func(t *testing.T) {
			base, ok := ScopeBase(c.module)
			if base != c.base || ok != c.ok {
				t.Fatalf("ScopeBase(%q) = (%q, %v), want (%q, %v)",
					c.module, base, ok, c.base, c.ok)
			}
		})
	}
}

func TestScopeBase_Unknown(t *testing.T) {
	cases := []string{
		"unknown-module",
		"",        // empty input
		"billing", // aggregate parent has no top-level entry
		"shop",
	}
	for _, m := range cases {
		t.Run("input="+m, func(t *testing.T) {
			base, ok := ScopeBase(m)
			if ok || base != "" {
				t.Fatalf("ScopeBase(%q) = (%q, %v), want (\"\", false)", m, base, ok)
			}
		})
	}
}

func TestScopeBase_LongestPrefix(t *testing.T) {
	// shop.articles.subnode is not in the map, but shop.articles is →
	// the walk should find shop.articles (collection).
	if base, ok := ScopeBase("shop.articles.subnode"); !ok || base != "collection" {
		t.Fatalf("walk-back: got (%q, %v), want (collection, true)", base, ok)
	}
	// shop.files.deep walks back to shop.files ("", but known).
	if base, ok := ScopeBase("shop.files.deep"); !ok || base != "" {
		t.Fatalf("walk-back to no-scope: got (%q, %v), want (\"\", true)", base, ok)
	}
}

func TestModuleScopes_KnownModule(t *testing.T) {
	got := ModuleScopes("products")
	if len(got) != 2 || got[0] != "read_product" || got[1] != "write_product" {
		t.Errorf("ModuleScopes(products) = %v, want [read_product write_product]", got)
	}
}

func TestModuleScopes_NoScopeModule(t *testing.T) {
	if got := ModuleScopes("webhook"); got != nil {
		t.Errorf("ModuleScopes(webhook) = %v, want nil", got)
	}
}

func TestModuleScopes_UnknownModule(t *testing.T) {
	if got := ModuleScopes("does-not-exist"); got != nil {
		t.Errorf("ModuleScopes(does-not-exist) = %v, want nil", got)
	}
}

func TestReadWriteScope(t *testing.T) {
	if got, ok := ReadScope("products"); !ok || got != "read_product" {
		t.Errorf("ReadScope(products) = (%q, %v), want (read_product, true)", got, ok)
	}
	if got, ok := WriteScope("products"); !ok || got != "write_product" {
		t.Errorf("WriteScope(products) = (%q, %v), want (write_product, true)", got, ok)
	}
	// No-scope module → both return ok=false.
	if got, ok := ReadScope("webhook"); ok || got != "" {
		t.Errorf("ReadScope(webhook) = (%q, %v), want (\"\", false)", got, ok)
	}
	if got, ok := WriteScope("webhook"); ok || got != "" {
		t.Errorf("WriteScope(webhook) = (%q, %v), want (\"\", false)", got, ok)
	}
	// Unknown → both return ok=false.
	if _, ok := ReadScope("typo"); ok {
		t.Errorf("ReadScope(typo) should report unknown")
	}
}

// TestEveryDerivedScopeIsInKnownScopes guards against typos in scope_map.json
// that would silently mint OAuth scope strings the platform doesn't accept.
func TestEveryDerivedScopeIsInKnownScopes(t *testing.T) {
	for module, base := range moduleScopeBase {
		if base == "" {
			continue
		}
		readScope := "read_" + base
		writeScope := "write_" + base
		if _, ok := knownScopes[readScope]; !ok {
			t.Errorf("module %q base %q produces %q which is not in knownScopes",
				module, base, readScope)
		}
		if _, ok := knownScopes[writeScope]; !ok {
			t.Errorf("module %q base %q produces %q which is not in knownScopes",
				module, base, writeScope)
		}
	}
}

// TestScopeMapCoversAllSpecModules ensures the scope map and the embedded
// cli_meta spec don't drift. Every spec module must appear (either bare or
// as the top component of a dotted key), and every scope-map key's top
// component must be a real spec module.
func TestScopeMapCoversAllSpecModules(t *testing.T) {
	spec := registry.LoadSpec()
	if len(spec.Modules) == 0 {
		t.Skip("embedded spec has no modules — skipping coverage test")
	}

	// Spec modules indexed for cheap lookup.
	specModules := map[string]struct{}{}
	for _, m := range spec.Modules {
		specModules[m.Name] = struct{}{}
	}

	// Direction 1: every spec module appears in the scope map (bare or
	// as the top component of at least one dotted key).
	for name := range specModules {
		if _, hasBare := moduleScopeBase[name]; hasBare {
			continue
		}
		found := false
		for key := range moduleScopeBase {
			if strings.HasPrefix(key, name+".") {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("spec module %q has no scope_map.json entry", name)
		}
	}

	// extraDomains are CLI-convenience --domain names intentionally present in
	// scope_map.json that don't correspond to a registry spec module. `checkout`
	// is a hardcoded top-level command (not spec-driven) whose extension API is
	// gated by the themes scope; it's mapped here so `auth login --domain checkout`
	// is discoverable and grants read/write_themes.
	extraDomains := map[string]struct{}{"checkout": {}}

	// Direction 2: every map key's top component is a real spec module (or a
	// documented extra domain).
	for key := range moduleScopeBase {
		top := key
		if i := strings.Index(key, "."); i >= 0 {
			top = key[:i]
		}
		if _, ok := specModules[top]; ok {
			continue
		}
		if _, extra := extraDomains[top]; extra {
			continue
		}
		t.Errorf("scope_map.json key %q references unknown spec module %q", key, top)
	}
}

func TestExpandDomain(t *testing.T) {
	// Parent module with children: union of parent scope + child scopes.
	// products' distinct bases: {product, collection, comments, inventory,
	// gift_cards} → 10 final scopes.
	got, err := ExpandDomain("products")
	if err != nil {
		t.Fatalf("ExpandDomain(products) err = %v", err)
	}
	want := []string{
		"read_collection", "read_comments", "read_gift_cards", "read_inventory", "read_product",
		"write_collection", "write_comments", "write_gift_cards", "write_inventory", "write_product",
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandDomain(products) = %v, want %v", got, want)
	}

	// Leaf module: discounts.coupons base = price_rules (same as parent).
	got, err = ExpandDomain("discounts")
	if err != nil {
		t.Fatalf("ExpandDomain(discounts) err = %v", err)
	}
	wantDiscounts := []string{"read_price_rules", "write_price_rules"}
	if !reflect.DeepEqual(got, wantDiscounts) {
		t.Fatalf("ExpandDomain(discounts) = %v, want %v", got, wantDiscounts)
	}

	// Aggregate: after themes migrated to its own top-level module, shop has
	// 11 children whose distinct bases are {shop, collection, shop_navigation,
	// product} (4 bases) plus no-scope children. Result: 8 final scopes.
	got, err = ExpandDomain("shop")
	if err != nil {
		t.Fatalf("ExpandDomain(shop) err = %v", err)
	}
	wantShop := []string{
		"read_collection", "read_product", "read_shop", "read_shop_navigation",
		"write_collection", "write_product", "write_shop", "write_shop_navigation",
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantShop) {
		t.Fatalf("ExpandDomain(shop) = %v, want %v", got, wantShop)
	}

	// themes is its own top-level module with the dedicated "themes" scope base.
	// It ALSO implies read_shop (read-only): `themes serve`/`themes share` call
	// GET /shop to build the preview-URL banner, so a themes-only login must be
	// able to read shop info — but NOT write it (no write_shop).
	got, err = ExpandDomain("themes")
	if err != nil {
		t.Fatalf("ExpandDomain(themes) err = %v", err)
	}
	wantThemes := []string{"read_shop", "read_themes", "write_themes"}
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantThemes) {
		t.Fatalf("ExpandDomain(themes) = %v, want %v", got, wantThemes)
	}
	for _, s := range got {
		if s == "write_shop" {
			t.Fatalf("ExpandDomain(themes) must NOT grant write_shop (read-only shop access); got %v", got)
		}
	}

	// Aggregate where ALL children have no scope.
	// billing.* are all anonymous → (nil, nil) via aggregate-prefix fast path.
	got, err = ExpandDomain("billing")
	if err != nil || got != nil {
		t.Fatalf("ExpandDomain(billing) = (%v, %v), want (nil, nil)", got, err)
	}

	// No-scope leaf → (nil, nil)
	got, err = ExpandDomain("webhook")
	if err != nil || got != nil {
		t.Fatalf("ExpandDomain(webhook) = (%v, %v), want (nil, nil)", got, err)
	}

	// Unknown name → error
	if _, err := ExpandDomain("does-not-exist"); err == nil {
		t.Fatalf("ExpandDomain(does-not-exist) expected error")
	}

	// Empty string → error
	if _, err := ExpandDomain(""); err == nil {
		t.Fatalf("ExpandDomain(\"\") expected error")
	}
}

func TestExpandDomain_All(t *testing.T) {
	got, err := ExpandDomain(DomainAll)
	if err != nil {
		t.Fatalf("ExpandDomain(all) err = %v", err)
	}
	// Result should be sorted and contain at least the well-known read/write
	// pairs from a few sentinel bases. We don't enumerate the full union here
	// — drift tests in TestScopeMapCoversAllSpecModules cover completeness.
	for _, want := range []string{
		"read_product", "write_product",
		"read_order", "write_order",
		"read_collection", "write_collection",
		"read_price_rules", "write_price_rules",
	} {
		if !contains(got, want) {
			t.Errorf("ExpandDomain(all) missing %q (got %d scopes)", want, len(got))
		}
	}
	// Result should be deduped — no scope appears twice.
	seen := map[string]struct{}{}
	for _, s := range got {
		if _, dup := seen[s]; dup {
			t.Errorf("ExpandDomain(all) returned duplicate scope %q", s)
		}
		seen[s] = struct{}{}
	}
	// Result should be sorted.
	for i := 1; i < len(got); i++ {
		if got[i] < got[i-1] {
			t.Errorf("ExpandDomain(all) not sorted: %q before %q", got[i-1], got[i])
		}
	}
}

func TestTopLevelDomains_NoDottedPaths(t *testing.T) {
	got := TopLevelDomains()
	for _, d := range got {
		if strings.Contains(d, ".") {
			t.Errorf("TopLevelDomains contains dotted path %q", d)
		}
	}
	// Sanity: should include at least a few known top-levels.
	for _, want := range []string{"products", "orders", "billing", "shop"} {
		if !contains(got, want) {
			t.Errorf("TopLevelDomains missing %q", want)
		}
	}
}

func contains(haystack []string, needle string) bool {
	for _, s := range haystack {
		if s == needle {
			return true
		}
	}
	return false
}

func TestSupportedDomains_Includes(t *testing.T) {
	got := SupportedDomains()
	idx := map[string]struct{}{}
	for _, d := range got {
		idx[d] = struct{}{}
	}
	for _, want := range []string{"products", "products.comments", "shop", "shop.blogs", "billing.usage-charges"} {
		if _, ok := idx[want]; !ok {
			t.Errorf("SupportedDomains missing %q", want)
		}
	}
	// Should not contain unknown items.
	if _, ok := idx["does-not-exist"]; ok {
		t.Error("SupportedDomains leaked unknown entry")
	}
}

// TestScopeMapJSONIsValid is a tiny smoke that catches commit-time format
// issues (missing comma, stray comment, etc.) that init()'s panic would also
// catch — but as a test it's easier to debug.
func TestScopeMapJSONIsValid(t *testing.T) {
	var probe map[string]scopeEntry
	if err := json.Unmarshal(scopeMapJSON, &probe); err != nil {
		t.Fatalf("scope_map.json is invalid JSON: %v", err)
	}
	if len(probe) == 0 {
		t.Fatal("scope_map.json parsed to empty map")
	}
}

func TestExpandDomains_Empty(t *testing.T) {
	got, err := ExpandDomains(nil)
	if err != nil || got != nil {
		t.Fatalf("ExpandDomains(nil) = (%v, %v), want (nil, nil)", got, err)
	}
	got, err = ExpandDomains([]string{})
	if err != nil || got != nil {
		t.Fatalf("ExpandDomains([]) = (%v, %v), want (nil, nil)", got, err)
	}
}

func TestExpandDomains_LeafModule(t *testing.T) {
	got, err := ExpandDomains([]string{"customers"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// customers + customers.addresses both → customer.
	want := []string{"read_customer", "write_customer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandDomains_AggregateShop(t *testing.T) {
	got, err := ExpandDomains([]string{"shop"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// shop's 11 children (themes migrated out) dedupe to bases {shop,
	// collection, shop_navigation, product} → 8 final OAuth scope strings.
	wantSorted := []string{
		"read_collection", "read_product", "read_shop", "read_shop_navigation",
		"write_collection", "write_product", "write_shop", "write_shop_navigation",
	}
	sort.Strings(got)
	if !reflect.DeepEqual(got, wantSorted) {
		t.Fatalf("got %v, want %v", got, wantSorted)
	}
}

func TestExpandDomains_MultipleDomainsDedupe(t *testing.T) {
	// customers and customers.addresses both → customer → same read/write
	// pair, must dedupe.
	got, err := ExpandDomains([]string{"customers", "customers.addresses"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"read_customer", "write_customer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v (duplicates should be removed)", got, want)
	}
}

func TestExpandDomains_NoScopeContributesNothing(t *testing.T) {
	// webhook is known but contributes no scope. Combined with customers,
	// only the customer scopes appear.
	got, err := ExpandDomains([]string{"customers", "webhook"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	want := []string{"read_customer", "write_customer"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("got %v, want %v", got, want)
	}
}

func TestExpandDomains_AllSentinel(t *testing.T) {
	got, err := ExpandDomains([]string{"all"})
	if err != nil {
		t.Fatalf("err = %v", err)
	}
	// Should include at least the major bases from scope_map.json.
	for _, want := range []string{
		"read_product", "write_product",
		"read_order", "write_order",
		"read_collection", "write_collection",
		"read_price_rules", "write_price_rules",
	} {
		found := false
		for _, s := range got {
			if s == want {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("--domain all missing %q (got %d scopes)", want, len(got))
		}
	}
}

func TestExpandDomains_AllDedupesWithExplicitDomain(t *testing.T) {
	// --domain all,products — products' scopes are already inside "all",
	// so the dedupe path should produce the same length as "all" alone.
	allOnly, _ := ExpandDomains([]string{"all"})
	allPlusProducts, _ := ExpandDomains([]string{"all", "products"})
	if len(allOnly) != len(allPlusProducts) {
		t.Errorf("all,products len = %d, want %d (dedupe failed)",
			len(allPlusProducts), len(allOnly))
	}
}

func TestExpandDomains_UnknownReturnsError(t *testing.T) {
	_, err := ExpandDomains([]string{"products", "typo"})
	if err == nil {
		t.Fatal("expected error for unknown domain")
	}
	if !strings.Contains(err.Error(), "typo") {
		t.Errorf("error should name the offending domain, got %v", err)
	}
}

func TestDedupePreserveOrder(t *testing.T) {
	cases := []struct {
		in   []string
		want []string
	}{
		{nil, nil},
		{[]string{}, nil},
		{[]string{"a"}, []string{"a"}},
		{[]string{"a", "b", "a"}, []string{"a", "b"}},
		{[]string{"a", "b", "b", "c", "a"}, []string{"a", "b", "c"}},
	}
	for _, c := range cases {
		got := DedupePreserveOrder(c.in)
		if !reflect.DeepEqual(got, c.want) {
			t.Errorf("DedupePreserveOrder(%v) = %v, want %v", c.in, got, c.want)
		}
	}
}

// TestExpandDomain_Checkout locks the checkout domain (defined in
// scope_map.json as a themes-scoped leaf): checkout extension APIs are gated by
// the themes permission (checkout commands never call /shop), so --domain
// checkout grants [read_themes, write_themes], and "checkout" must be a listed,
// discoverable domain.
func TestExpandDomain_Checkout(t *testing.T) {
	got, err := ExpandDomain("checkout")
	if err != nil {
		t.Fatalf("ExpandDomain(checkout): %v", err)
	}
	want := []string{"read_themes", "write_themes"}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("ExpandDomain(checkout) = %v, want %v", got, want)
	}
	// checkout must be discoverable in the domain lists (help text + validation).
	listed := false
	for _, d := range TopLevelDomains() {
		if d == "checkout" {
			listed = true
		}
	}
	if !listed {
		t.Error("checkout not in TopLevelDomains()")
	}
}
