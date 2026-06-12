package jsbuild

import "testing"

func TestParseNodeVersion(t *testing.T) {
	cases := []struct {
		in            string
		maj, min, pat int
		wantErr       bool
	}{
		{"v14.18.0\n", 14, 18, 0, false},
		{"v22.1.3", 22, 1, 3, false},
		{"v20.10.0-nightly20231012", 20, 10, 0, false}, // prerelease stripped
		{"16.0.0", 16, 0, 0, false},
		{"not-a-version", 0, 0, 0, true},
		{"", 0, 0, 0, true},
	}
	for _, c := range cases {
		maj, min, pat, err := parseNodeVersion(c.in)
		if c.wantErr {
			if err == nil {
				t.Errorf("parseNodeVersion(%q): expected error, got none", c.in)
			}
			continue
		}
		if err != nil {
			t.Errorf("parseNodeVersion(%q): %v", c.in, err)
			continue
		}
		if maj != c.maj || min != c.min || pat != c.pat {
			t.Errorf("parseNodeVersion(%q) = %d.%d.%d, want %d.%d.%d", c.in, maj, min, pat, c.maj, c.min, c.pat)
		}
	}
}

func TestNodeVersionMeetsFloor(t *testing.T) {
	cases := []struct {
		in   string
		want bool
	}{
		{"v14.18.0", true},
		{"v14.19.0", true},
		{"v14.9.0", false}, // <-- the string-compare landmine: "14.9.0" > "14.18.0" lexically
		{"v13.0.0", false},
		{"v16.0.0", true},
		{"v22.1.0", true},
	}
	for _, c := range cases {
		got, err := nodeVersionMeetsFloor(c.in)
		if err != nil {
			t.Fatalf("nodeVersionMeetsFloor(%q): %v", c.in, err)
		}
		if got != c.want {
			t.Errorf("nodeVersionMeetsFloor(%q) = %v, want %v", c.in, got, c.want)
		}
	}
}
