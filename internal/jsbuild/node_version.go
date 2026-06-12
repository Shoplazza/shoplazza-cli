package jsbuild

import (
	"fmt"
	"strconv"
	"strings"
)

// Minimum supported Node version (Vite 4.3.x official floor).
const (
	nodeMinMajor = 14
	nodeMinMinor = 18
	nodeMinPatch = 0
)

// parseNodeVersion parses `node --version` output ("vX.Y.Z\n", possibly with a
// "-prerelease"/"+build" suffix) into three integers. NUMERIC, not string.
func parseNodeVersion(out string) (major, minor, patch int, err error) {
	s := strings.TrimSpace(out)
	s = strings.TrimPrefix(s, "v")
	if i := strings.IndexAny(s, "-+"); i >= 0 {
		s = s[:i]
	}
	parts := strings.SplitN(s, ".", 3)
	if len(parts) < 3 {
		return 0, 0, 0, fmt.Errorf("unrecognized node version '%s'", strings.TrimSpace(out))
	}
	nums := make([]int, 3)
	for i := 0; i < 3; i++ {
		n, convErr := strconv.Atoi(strings.TrimSpace(parts[i]))
		if convErr != nil {
			return 0, 0, 0, fmt.Errorf("unrecognized node version '%s'", strings.TrimSpace(out))
		}
		nums[i] = n
	}
	return nums[0], nums[1], nums[2], nil
}

// nodeVersionMeetsFloor reports whether the version string is >= 14.18.0,
// comparing major/minor/patch as integers.
func nodeVersionMeetsFloor(out string) (bool, error) {
	maj, min, pat, err := parseNodeVersion(out)
	if err != nil {
		return false, err
	}
	if maj != nodeMinMajor {
		return maj > nodeMinMajor, nil
	}
	if min != nodeMinMinor {
		return min > nodeMinMinor, nil
	}
	return pat >= nodeMinPatch, nil
}
