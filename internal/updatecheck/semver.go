package updatecheck

import (
	"strconv"
	"strings"
)

// IsNewer reports whether version a is newer than b. Versions are strict "X.Y.Z" three numeric parts (optional v prefix).
// a unparseable -> false (cannot confirm update); b unparseable -> true (local version treated as stale).
func IsNewer(a, b string) bool {
	ap := parseVersion(a)
	bp := parseVersion(b)
	if ap == nil {
		return false
	}
	if bp == nil {
		return true
	}
	for i := range 3 {
		if ap[i] != bp[i] {
			return ap[i] > bp[i]
		}
	}
	return false
}

// parseVersion parses "X.Y.Z" (optional v prefix) into [major, minor, patch].
// Any other form (dev/git-describe builds, prerelease, wrong number of parts, non-numeric) returns nil,
// which the caller treats as a non-released version.
func parseVersion(v string) *[3]int {
	parts := strings.Split(strings.TrimPrefix(v, "v"), ".")
	if len(parts) != 3 {
		return nil
	}
	var out [3]int
	for i, p := range parts {
		n, err := strconv.Atoi(p)
		if err != nil || n < 0 {
			return nil
		}
		out[i] = n
	}
	return &out
}

// isReleaseVersion reports whether v is a released version (strict X.Y.Z, optional v prefix).
// dev/git-describe builds and any other non-three-numeric-part strings fail to parse and are treated as non-release.
func isReleaseVersion(v string) bool {
	return parseVersion(v) != nil
}
