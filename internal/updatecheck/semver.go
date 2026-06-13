package updatecheck

import (
	"regexp"
	"strconv"
	"strings"
)

// IsNewer 报告 a 是否比 b 新(semver)。两者都可解析则比 major/minor/patch,
// 相等再比 prerelease(prerelease < 正式版)。a 不可解析→false(无法确认更新);
// b 不可解析→true(本地版本视为过时)。
func IsNewer(a, b string) bool {
	ap := parseVersion(a)
	bp := parseVersion(b)
	if ap == nil {
		return false
	}
	if bp == nil {
		return true
	}
	for i := 0; i < 3; i++ {
		if ap.core[i] != bp.core[i] {
			return ap.core[i] > bp.core[i]
		}
	}
	return comparePrerelease(ap.prerelease, bp.prerelease) > 0
}

type parsedVersion struct {
	core       [3]int
	prerelease string
}

// prereleaseRe 校验 semver prerelease 标识(点分)。
var prereleaseRe = regexp.MustCompile(
	`^(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*)` +
		`(?:\.(?:0|[1-9]\d*|[0-9]*[a-zA-Z-][0-9a-zA-Z-]*))*$`)

// parseVersion 解析 "X.Y.Z"(可带 v 前缀、+build 元数据、-prerelease 后缀),非法返回 nil。
func parseVersion(v string) *parsedVersion {
	v = strings.TrimPrefix(v, "v")
	if i := strings.IndexByte(v, '+'); i >= 0 {
		v = v[:i]
	}
	prerelease := ""
	if i := strings.IndexByte(v, '-'); i >= 0 {
		prerelease = v[i+1:]
		v = v[:i]
		if prerelease == "" || !prereleaseRe.MatchString(prerelease) {
			return nil
		}
	}
	parts := strings.SplitN(v, ".", 3)
	if len(parts) != 3 {
		return nil
	}
	var core [3]int
	for i, p := range parts {
		if len(p) > 1 && p[0] == '0' {
			return nil // 前导零
		}
		n, err := strconv.Atoi(p)
		if err != nil {
			return nil
		}
		core[i] = n
	}
	return &parsedVersion{core: core, prerelease: prerelease}
}

// comparePrerelease 返回 -1/0/1。空 prerelease(正式版)排序更高。
func comparePrerelease(a, b string) int {
	if a == b {
		return 0
	}
	if a == "" {
		return 1
	}
	if b == "" {
		return -1
	}
	ai := strings.Split(a, ".")
	bi := strings.Split(b, ".")
	for i := 0; i < len(ai) && i < len(bi); i++ {
		if c := comparePrereleaseIdent(ai[i], bi[i]); c != 0 {
			return c
		}
	}
	switch {
	case len(ai) > len(bi):
		return 1
	case len(ai) < len(bi):
		return -1
	default:
		return 0
	}
}

func comparePrereleaseIdent(a, b string) int {
	an, aErr := strconv.Atoi(a)
	bn, bErr := strconv.Atoi(b)
	switch {
	case aErr == nil && bErr == nil:
		switch {
		case an > bn:
			return 1
		case an < bn:
			return -1
		default:
			return 0
		}
	case aErr == nil: // 数字 < 字母数字
		return -1
	case bErr == nil:
		return 1
	default:
		return strings.Compare(a, b)
	}
}

// gitDescribeRe 匹配 git describe dev 构建后缀,如 "-12-gabc1234"。
var gitDescribeRe = regexp.MustCompile(`-\d+-g[0-9a-f]{7,}`)

// isReleaseVersion 报告 v 是否为干净的已发布版本(semver,或 npm prerelease),
// 而非 git-describe dev 构建(如 "2.0.1-12-gabc1234[-dirty]")。
func isReleaseVersion(v string) bool {
	if parseVersion(v) == nil {
		return false
	}
	return !gitDescribeRe.MatchString(strings.TrimPrefix(v, "v"))
}
