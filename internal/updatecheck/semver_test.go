package updatecheck

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.0.1", "2.0.0", true},
		{"2.0.10", "2.0.9", true}, // compared as numbers, not strings
		{"2.1.0", "2.0.9", true},
		{"3.0.0", "2.9.9", true},
		{"2.0.0", "2.0.0", false},
		{"2.0.0", "2.0.1", false},
		{"v2.0.1", "2.0.0", true},             // v prefix
		{"2.0.1", "v2.0.0", true},             // mixed v prefix on both sides
		{"v2.2.2", "v2.2.1", true},            // both have v prefix
		{"garbage", "2.0.0", false},           // a unparseable -> false
		{"2.0.0", "garbage", true},            // b unparseable -> true
		{"2.0.1-12-gabc1234", "2.0.0", false}, // dev build (a) unparseable -> false
		{"2.1.0", "2.0.1-12-gabc1234", true},  // dev build (b) unparseable -> true
	}
	for _, c := range cases {
		if got := IsNewer(c.a, c.b); got != c.want {
			t.Errorf("IsNewer(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestIsReleaseVersion(t *testing.T) {
	// Only strict X.Y.Z (optional v prefix) is a release.
	for _, v := range []string{"2.0.1", "v2.0.1", "2.2.2", "v10.20.30"} {
		if !isReleaseVersion(v) {
			t.Errorf("isReleaseVersion(%q)=false want true", v)
		}
	}
	// dev / git-describe / prerelease / wrong part count / non-numeric are not releases.
	for _, v := range []string{"dev", "", "2.0.1-12-gabc1234", "2.0.1-dirty", "2.1.0-beta.1", "not.a.version", "2.0", "2.0.0.1"} {
		if isReleaseVersion(v) {
			t.Errorf("isReleaseVersion(%q)=true want false", v)
		}
	}
}
