package dynamic

import (
	"testing"
)

func TestFirstNonEmpty(t *testing.T) {
	cases := []struct {
		values []string
		want   string
	}{
		{[]string{"", "", "third"}, "third"},
		{[]string{"first", "second"}, "first"},
		{[]string{"", ""}, ""},
		{nil, ""},
	}
	for _, c := range cases {
		if got := firstNonEmpty(c.values...); got != c.want {
			t.Errorf("firstNonEmpty(%v) = %q, want %q", c.values, got, c.want)
		}
	}
}

func TestTitleCase(t *testing.T) {
	cases := []struct {
		input string
		want  string
	}{
		{"gift-cards", "Gift Cards"},
		{"orders", "Orders"},
		{"shop-blogs", "Shop Blogs"},
		{"", ""},
		{"single", "Single"},
	}
	for _, c := range cases {
		if got := titleCase(c.input); got != c.want {
			t.Errorf("titleCase(%q) = %q, want %q", c.input, got, c.want)
		}
	}
}
