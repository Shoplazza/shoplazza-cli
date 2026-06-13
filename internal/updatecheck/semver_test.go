package updatecheck

import "testing"

func TestIsNewer(t *testing.T) {
	cases := []struct {
		a, b string
		want bool
	}{
		{"2.0.1", "2.0.0", true},
		{"2.0.10", "2.0.9", true}, // 按数字段比,不是字符串
		{"2.1.0", "2.0.9", true},
		{"3.0.0", "2.9.9", true},
		{"2.0.0", "2.0.0", false},
		{"2.0.0", "2.0.1", false},
		{"v2.0.1", "2.0.0", true},             // v 前缀
		{"2.0.1", "v2.0.0", true},             // 两侧 v 前缀混用
		{"v2.2.2", "v2.2.1", true},            // 都带 v
		{"garbage", "2.0.0", false},           // a 不可解析 -> false
		{"2.0.0", "garbage", true},            // b 不可解析 -> true
		{"2.0.1-12-gabc1234", "2.0.0", false}, // dev 构建(a)不可解析 -> false
		{"2.1.0", "2.0.1-12-gabc1234", true},  // dev 构建(b)不可解析 -> true
	}
	for _, c := range cases {
		if got := IsNewer(c.a, c.b); got != c.want {
			t.Errorf("IsNewer(%q,%q)=%v want %v", c.a, c.b, got, c.want)
		}
	}
}

func TestIsReleaseVersion(t *testing.T) {
	// 纯 X.Y.Z(可带 v 前缀)才是 release。
	for _, v := range []string{"2.0.1", "v2.0.1", "2.2.2", "v10.20.30"} {
		if !isReleaseVersion(v) {
			t.Errorf("isReleaseVersion(%q)=false want true", v)
		}
	}
	// dev / git-describe / prerelease / 段数不对 / 非数字 都不是 release。
	for _, v := range []string{"dev", "", "2.0.1-12-gabc1234", "2.0.1-dirty", "2.1.0-beta.1", "not.a.version", "2.0", "2.0.0.1"} {
		if isReleaseVersion(v) {
			t.Errorf("isReleaseVersion(%q)=true want false", v)
		}
	}
}
