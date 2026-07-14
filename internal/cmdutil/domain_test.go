package cmdutil

import "testing"

func TestNormalizeStoreDomain(t *testing.T) {
	cases := map[string]string{
		"x.com":            "x.com",
		"https://x.com":    "x.com",
		"http://x.com":     "x.com",
		"https://x.com/":   "x.com",
		"x.com/":           "x.com",
		" https://x.com/ ": "x.com",
		// Scheme strip must be case-insensitive but preserve the domain's case.
		"HTTPS://x.com":        "x.com",
		"HTTP://x.com":         "x.com",
		"HtTpS://MyStore.com/": "MyStore.com",
	}
	for in, want := range cases {
		if got := NormalizeStoreDomain(in); got != want {
			t.Errorf("NormalizeStoreDomain(%q) = %q, want %q", in, got, want)
		}
	}
}
