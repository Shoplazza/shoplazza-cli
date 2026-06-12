package theme_extension

import "testing"

func TestValidVersionFormat(t *testing.T) {
	for _, v := range []string{"1.0.0", "0.0.1", "10.20.30"} {
		if !ValidVersionFormat(v) {
			t.Errorf("ValidVersionFormat(%q) = false, want true", v)
		}
	}
	for _, v := range []string{"", "1.0", "1.0.0.0", "1.0.0-beta", "v1.0.0", "1.0.x", "1..0"} {
		if ValidVersionFormat(v) {
			t.Errorf("ValidVersionFormat(%q) = true, want false", v)
		}
	}
}

func TestCompareVersions(t *testing.T) {
	cases := []struct {
		a, b string
		want int
	}{
		{"1.0.1", "1.0.0", 1},
		{"1.1.0", "1.0.9", 1},
		{"2.0.0", "1.9.9", 1},
		{"1.0.0", "1.0.0", 0},
		{"1.0.0", "1.0.1", -1},
		{"1.0.0", "1.1.0", -1},
		{"1.2.0", "1.2", 0}, // missing segment treated as 0
		// unparseable segments fold to 0 — the comparison stays total; callers
		// needing a confirmed ordering pre-validate with ValidVersionFormat
		{"1.0.1", "1.0.0-beta", 1},
		{"1.0.0", "1.0.x", 0},
		{"garbage", "0.0.0", 0},
	}
	for _, c := range cases {
		if got := CompareVersions(c.a, c.b); got != c.want {
			t.Errorf("CompareVersions(%q,%q) = %d, want %d", c.a, c.b, got, c.want)
		}
	}
}

func TestLatestVersion(t *testing.T) {
	if got := LatestVersion(nil); got != "" {
		t.Errorf("empty list → %q, want \"\"", got)
	}
	vers := []map[string]any{{"version": "2.0.0"}, {"version": "1.0.0"}}
	if got := LatestVersion(vers); got != "2.0.0" {
		t.Errorf("LatestVersion = %q, want 2.0.0 (index 0)", got)
	}
}

func TestVersionIDFor(t *testing.T) {
	vers := []map[string]any{
		{"version": "2.0.0", "version_id": "v2"},
		{"version": "1.0.0", "version_id": "v1"},
	}
	if id, ok := VersionIDFor(vers, "1.0.0"); !ok || id != "v1" {
		t.Errorf("VersionIDFor 1.0.0 = %q,%v; want v1,true", id, ok)
	}
	if id, ok := VersionIDFor(vers, "2.0.0"); !ok || id != "v2" {
		t.Errorf("VersionIDFor 2.0.0 = %q,%v; want v2,true", id, ok)
	}
	if _, ok := VersionIDFor(vers, "9.9.9"); ok {
		t.Error("VersionIDFor 9.9.9 should be not-found")
	}
	if _, ok := VersionIDFor(nil, "1.0.0"); ok {
		t.Error("VersionIDFor on empty list should be not-found")
	}
	// match with no version_id → not found
	if _, ok := VersionIDFor([]map[string]any{{"version": "1.0.0"}}, "1.0.0"); ok {
		t.Error("match without version_id should be not-found")
	}
}
