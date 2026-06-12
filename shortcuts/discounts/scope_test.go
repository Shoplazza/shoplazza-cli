package discounts

import (
	"reflect"
	"strings"
	"testing"
)

func TestResolveScope(t *testing.T) {
	cases := []struct {
		name         string
		products     []string
		collections  []string
		variants     []string
		exclude      bool
		requireScope bool
		names        scopeNames
		want         map[string]any
		wantErr      string // substring; "" = no error
	}{
		{
			name:     "products entitled",
			products: []string{"p1", "p2"},
			names:    defaultScopeNames(),
			want:     map[string]any{"selection": "entitled", "product_ids": []string{"p1", "p2"}},
		},
		{
			name:        "collections exclude",
			collections: []string{"c1"},
			exclude:     true,
			names:       defaultScopeNames(),
			want:        map[string]any{"selection": "exclude", "collection_ids": []string{"c1"}},
		},
		{
			name:     "variants entitled",
			variants: []string{"v1"},
			names:    defaultScopeNames(),
			want:     map[string]any{"selection": "entitled", "variant_ids": []string{"v1"}},
		},
		{
			name:  "empty optional scope -> all",
			names: defaultScopeNames(),
			want:  map[string]any{"selection": "all"},
		},
		{
			name:         "empty required scope -> error",
			requireScope: true,
			names:        defaultScopeNames(),
			wantErr:      "is required",
		},
		{
			name:    "exclude with empty scope -> error",
			exclude: true,
			names:   defaultScopeNames(),
			wantErr: "needs a scope",
		},
		{
			name:        "two lists set -> mutex error",
			products:    []string{"p1"},
			collections: []string{"c1"},
			names:       defaultScopeNames(),
			wantErr:     "mutually exclusive",
		},
		{
			name:        "flashsale names omit --products in mutex error",
			collections: []string{"c1"},
			variants:    []string{"v1"},
			names:       scopeNames{collections: "collections", variants: "variants", exclude: "exclude"},
			wantErr:     "--collections",
		},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got, err := resolveScope(tc.products, tc.collections, tc.variants, tc.exclude, tc.requireScope, tc.names)
			if tc.wantErr != "" {
				if err == nil {
					t.Fatalf("expected error containing %q, got nil (result=%v)", tc.wantErr, got)
				}
				if !strings.Contains(err.Error(), tc.wantErr) {
					t.Fatalf("error = %q, want substring %q", err.Error(), tc.wantErr)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(got, tc.want) {
				t.Fatalf("resolveScope = %#v, want %#v", got, tc.want)
			}
		})
	}
}

// Flashsale names must never name --products (it has no such flag).
func TestResolveScope_FlashsaleNamesNoProducts(t *testing.T) {
	names := scopeNames{collections: "collections", variants: "variants", exclude: "exclude"}
	_, err := resolveScope(nil, []string{"c1"}, []string{"v1"}, false, false, names)
	if err == nil {
		t.Fatal("expected mutex error")
	}
	if strings.Contains(err.Error(), "products") {
		t.Fatalf("flashsale mutex error must not mention --products: %q", err.Error())
	}
}
