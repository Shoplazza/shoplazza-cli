package build

import "testing"

func TestDisplayVersion(t *testing.T) {
	cases := []struct{ in, want string }{
		{"v26.0.0-406-gf191a84-dirty", "v26.0.0"},   // dev build, commits past tag + dirty
		{"v26.0.0-406-gf191a84", "v26.0.0"},         // dev build, clean
		{"v26.0.0-dirty", "v26.0.0"},                // exactly on tag, dirty tree
		{"v26.0.0", "v26.0.0"},                      // clean release build
		{"v26.0.0-rc.1-5-gabcdef0", "v26.0.0-rc.1"}, // hyphenated tag preserved
		{"dev", "dev"},                              // plain `go build` fallback
		{"f191a84-dirty", "f191a84"},                // --always, no tag: keep the hash
	}
	for _, c := range cases {
		old := Version
		Version = c.in
		got := DisplayVersion()
		Version = old
		if got != c.want {
			t.Errorf("DisplayVersion(%q) = %q, want %q", c.in, got, c.want)
		}
	}
}
