package updatecheck

import (
	"strconv"
	"strings"
)

// IsNewer 报告 a 是否比 b 新。版本号是纯 "X.Y.Z" 三段数字(可带 v 前缀)。
// a 不可解析→false(无法确认更新);b 不可解析→true(本地版本视为过时)。
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

// parseVersion 解析 "X.Y.Z"(可带 v 前缀)为 [major, minor, patch]。
// 任何其它形式(dev / git-describe 构建、prerelease、段数不对、非数字)都返回 nil,
// 调用方据此视为"非 release"。
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

// isReleaseVersion 报告 v 是否为已发布版本(纯 X.Y.Z,可带 v 前缀)。
// dev / git-describe 构建及其它任何非纯三段数字的字符串都解析失败,视为非 release。
func isReleaseVersion(v string) bool {
	return parseVersion(v) != nil
}
