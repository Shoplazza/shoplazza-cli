package common_test

import (
	"strings"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
)

func TestAutoName(t *testing.T) {
	name := common.AutoName("flashsale")
	if !strings.HasPrefix(name, "cli_flashsale_") {
		t.Errorf("AutoName = %q, want prefix cli_flashsale_", name)
	}
	suffix := strings.TrimPrefix(name, "cli_flashsale_")
	if suffix == "" {
		t.Errorf("AutoName has empty unix suffix: %q", name)
	}
}

func TestTruncateName(t *testing.T) {
	cases := []struct {
		input string
		n     int
		want  string
	}{
		{"hello", 10, "hello"},
		{"hello world", 5, "hello"},
		{"abcdefghij", 10, "abcdefghij"},
		{"abcdefghijk", 10, "abcdefghij"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		if got := common.TruncateName(tc.input, tc.n); got != tc.want {
			t.Errorf("TruncateName(%q, %d) = %q, want %q", tc.input, tc.n, got, tc.want)
		}
	}
}

func TestParseTime_Now(t *testing.T) {
	before := time.Now().Unix()
	ts, err := common.ParseTime("now")
	after := time.Now().Unix()
	if err != nil {
		t.Fatalf("ParseTime(now): %v", err)
	}
	if ts < before || ts > after {
		t.Errorf("ParseTime(now) = %d, want [%d, %d]", ts, before, after)
	}
}

func TestParseTime_Forever(t *testing.T) {
	for _, s := range []string{"forever", "-1"} {
		ts, err := common.ParseTime(s)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", s, err)
		}
		if ts != -1 {
			t.Errorf("ParseTime(%q) = %d, want -1", s, ts)
		}
	}
}

func TestParseTime_Relative(t *testing.T) {
	before := time.Now()
	cases := []struct {
		expr    string
		minSecs int64
		maxSecs int64
	}{
		{"+1h", 3500, 3700},
		{"+1d", 86300, 86500},
		{"+1w", 604700, 604900},
	}
	for _, tc := range cases {
		ts, err := common.ParseTime(tc.expr)
		if err != nil {
			t.Fatalf("ParseTime(%q): %v", tc.expr, err)
		}
		delta := ts - before.Unix()
		if delta < tc.minSecs || delta > tc.maxSecs {
			t.Errorf("ParseTime(%q) delta = %ds, want [%d, %d]", tc.expr, delta, tc.minSecs, tc.maxSecs)
		}
	}
}

func TestParseTime_AbsoluteDate(t *testing.T) {
	ts, err := common.ParseTime("2026-11-01")
	if err != nil {
		t.Fatalf("ParseTime(date): %v", err)
	}
	want, _ := time.ParseInLocation("2006-01-02", "2026-11-01", time.UTC)
	if ts != want.Unix() {
		t.Errorf("ParseTime(2026-11-01) = %d, want %d", ts, want.Unix())
	}
}

func TestParseTime_UnixInt(t *testing.T) {
	ts, err := common.ParseTime("1777564800")
	if err != nil {
		t.Fatalf("ParseTime(unix int): %v", err)
	}
	if ts != 1777564800 {
		t.Errorf("ParseTime(1777564800) = %d, want 1777564800", ts)
	}
}

func TestParseTime_Invalid(t *testing.T) {
	cases := []string{"+0d", "+abc", "not-a-date", "+5x"}
	for _, s := range cases {
		if _, err := common.ParseTime(s); err == nil {
			t.Errorf("ParseTime(%q) expected error, got nil", s)
		}
	}
}
