package discounts

import (
	"reflect"
	"testing"
)

func searchFlags() map[string]string {
	return map[string]string{
		"query": "string", "discount-code": "string",
		"progress": "stringslice", "discount-type": "stringslice",
		"discount-target": "stringslice", "discount-method": "stringslice",
		"page-limit": "int", "cursor": "string",
		"since": "string", "until": "string",
		"combines": "stringslice", "source": "stringslice", "ids": "stringslice",
	}
}

func TestSearchPlan(t *testing.T) {
	cases := []struct {
		name      string
		values    map[string]string
		want      map[string]any
		wantParam string
	}{
		{"defaults", nil, map[string]any{}, ""},
		{"paging", map[string]string{"page-limit": "5", "cursor": "c-2"}, map[string]any{"page_size": 5, "cursor": "c-2"}, ""},
		{"page-limit above 250", map[string]string{"page-limit": "500"}, nil, "--page-limit"},
		{"combines", map[string]string{"combines": "order,product"}, map[string]any{"discount_combines": []string{"order", "product"}}, ""},
		{"source names and ids", map[string]string{"source": "Manual,athena,other,42"}, map[string]any{"source_ids": []string{"1", "10", "999", "42"}}, ""},
		{"unknown source name", map[string]string{"source": "bogus"}, nil, "--source"},
		{"ids", map[string]string{"ids": "101,102"}, map[string]any{"ids": []string{"101", "102"}}, ""},
		{"--since alone leaves the upper bound open", map[string]string{"since": "2026-12-01"}, map[string]any{"starts_date": "2026-12-01T00:00:00Z", "ends_date": "-1"}, ""},
		{"--until alone leaves the lower bound open", map[string]string{"until": "2026-12-01T00:00:00+08:00"}, map[string]any{"starts_date": "-1", "ends_date": "2026-11-30T16:00:00Z"}, ""},
		{"--since after --until", map[string]string{"since": "2026-12-02", "until": "2026-12-01"}, nil, "--since"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			p, err := searchShortcut.Plan(newPlanInput(t, "search", searchFlags(), c.values))
			if c.wantParam != "" {
				wantValidation(t, err, c.wantParam)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(p.Query, c.want) {
				t.Errorf("query = %#v, want %#v", p.Query, c.want)
			}
		})
	}
}
