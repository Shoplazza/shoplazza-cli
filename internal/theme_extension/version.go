package theme_extension

import (
	"regexp"
	"strconv"
	"strings"
)

// semverRe is v1's strict X.Y.Z format: three numeric segments, nothing else.
var semverRe = regexp.MustCompile(`^[0-9]+\.[0-9]+\.[0-9]+$`)

// ValidVersionFormat reports whether v is X.Y.Z (numeric segments only).
func ValidVersionFormat(v string) bool {
	return semverRe.MatchString(v)
}

// CompareVersions returns -1, 0, or 1 as a<b, a==b, a>b, comparing dot-separated
// numeric segments left-to-right. A missing or unparseable segment counts as 0
// (deliberate: the comparison stays total) — callers that need a confirmed
// ordering must pre-validate both sides with ValidVersionFormat.
func CompareVersions(a, b string) int {
	as := strings.Split(a, ".")
	bs := strings.Split(b, ".")
	n := max(len(as), len(bs))
	for i := range n {
		var x, y int
		if i < len(as) {
			x = segmentValue(as[i])
		}
		if i < len(bs) {
			y = segmentValue(bs[i])
		}
		if x < y {
			return -1
		}
		if x > y {
			return 1
		}
	}
	return 0
}

// segmentValue parses one version segment; non-numeric → 0 (see CompareVersions).
func segmentValue(s string) int {
	n, err := strconv.Atoi(s)
	if err != nil {
		return 0
	}
	return n
}

// LatestVersion returns the "version" of the newest entry — index 0, since the
// versions API returns newest-first. Empty when the list is empty or the field
// is missing/non-string.
func LatestVersion(vers []map[string]any) string {
	if len(vers) == 0 {
		return ""
	}
	v, _ := vers[0]["version"].(string)
	return v
}

// VersionIDFor resolves a human semver value (e.g. "1.0.0") to its server
// version_id by scanning the version list. Versions are newest-first, so the
// first match wins (version values are unique per extension). Returns ok=false
// when no entry's "version" equals v or the match carries no "version_id".
func VersionIDFor(vers []map[string]any, v string) (string, bool) {
	for _, m := range vers {
		if mv, _ := m["version"].(string); mv == v {
			if id, _ := m["version_id"].(string); id != "" {
				return id, true
			}
		}
	}
	return "", false
}
