package discounts

import (
	"errors"
	"reflect"
	"testing"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
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

func wantValidation(t *testing.T, err error, param string) {
	t.Helper()
	var ee *output.ExitError
	if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Param != param {
		t.Fatalf("want a validation error on %s, got %#v", param, err)
	}
}

func TestSearchQuery(t *testing.T) {
	cases := []struct {
		name      string
		values    map[string]string
		want      map[string]any
		wantParam string
	}{
		{"defaults", nil, map[string]any{}, ""},
		{"paging", map[string]string{"page-limit": "5", "cursor": "c-2"}, map[string]any{"page_size": 5, "cursor": "c-2"}, ""},
		{"combines", map[string]string{"combines": "order,product"}, map[string]any{"discount_combines": []string{"order", "product"}}, ""},
		{"source names and ids", map[string]string{"source": "Manual,athena,other,42"}, map[string]any{"source_ids": []string{"1", "10", "999", "42"}}, ""},
		{"unknown source name", map[string]string{"source": "bogus"}, nil, "--source"},
		{"ids", map[string]string{"ids": "101,102"}, map[string]any{"ids": []string{"101", "102"}}, ""},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q, err := searchQuery(newPlanInput(t, "search", searchFlags(), c.values).Flags)
			if c.wantParam != "" {
				wantValidation(t, err, c.wantParam)
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(q, c.want) {
				t.Errorf("query = %#v, want %#v", q, c.want)
			}
		})
	}
}

func TestSearchPlanWindow(t *testing.T) {
	cases := []struct {
		name               string
		values             map[string]string
		wantStart, wantEnd any
		wantParam          string
	}{
		{"neither", nil, nil, nil, ""},
		{"--since alone leaves the upper bound open", map[string]string{"since": "2026-12-01"}, "2026-12-01T00:00:00Z", "-1", ""},
		{"--until alone leaves the lower bound open", map[string]string{"until": "2026-12-01T00:00:00+08:00"}, "-1", "2026-11-30T16:00:00Z", ""},
		{"--since after --until", map[string]string{"since": "2026-12-02", "until": "2026-12-01"}, nil, nil, "--since"},
		{"invalid --until", map[string]string{"until": "soon"}, nil, nil, "--until"},
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
			if p.Query["starts_date"] != c.wantStart || p.Query["ends_date"] != c.wantEnd {
				t.Errorf("starts_date/ends_date = %v/%v, want %v/%v", p.Query["starts_date"], p.Query["ends_date"], c.wantStart, c.wantEnd)
			}
		})
	}
}
