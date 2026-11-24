package common_test

import (
	"errors"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/common"
	"github.com/Shoplazza/shoplazza-cli/v2/shortcuts/internal/shortcuttest"
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

func TestParseTimeAt(t *testing.T) {
	// 02:30 in UTC+8 is 18:30 the previous day in UTC, so today/yesterday must follow the UTC date.
	now := time.Date(2026, 11, 24, 2, 30, 0, 0, time.FixedZone("UTC+8", 8*3600))
	cases := []struct{ in, want string }{
		{"now", "2026-11-23T18:30:00Z"},
		{"today", "2026-11-23T00:00:00Z"},
		{"Yesterday", "2026-11-22T00:00:00Z"},
		{"+12h", "2026-11-24T06:30:00Z"},
		{"-7d", "2026-11-16T18:30:00Z"},
		{"+2w", "2026-12-07T18:30:00Z"},
		{"2026-11-01", "2026-11-01T00:00:00Z"},
		{"2027-06-30 16:00", "2027-06-30T16:00:00Z"},
		{"2027-06-30T16:00", "2027-06-30T16:00:00Z"},
		{"2027-06-30 16:00:05", "2027-06-30T16:00:05Z"},
		{"2027-06-30T16:00:00", "2027-06-30T16:00:00Z"},
		{"2027-06-30T16:00:00Z", "2027-06-30T16:00:00Z"},
		{"2027-06-30T16:00:00.123Z", "2027-06-30T16:00:00.123Z"},
		{"2027-06-30T16:00:00+08:00", "2027-06-30T08:00:00Z"},
		{"2027-06-30 16:00+08:00", "2027-06-30T08:00:00Z"},
		{"2027-06-30t16:00:00z", "2027-06-30T16:00:00Z"},
		{"2027-06-30T16:00:00+0800", "2027-06-30T08:00:00Z"},
		{"2027-06-30 16:00 +08:00", "2027-06-30T08:00:00Z"},
		{"2027-06-30 16:00:00 +0800", "2027-06-30T08:00:00Z"},
		{" 1700000000 ", "2023-11-14T22:13:20Z"},
	}
	for _, c := range cases {
		got, err := common.ParseTimeAt(c.in, now)
		if err != nil {
			t.Errorf("ParseTimeAt(%q): %v", c.in, err)
			continue
		}
		if s := got.UTC().Format(time.RFC3339Nano); s != c.want {
			t.Errorf("ParseTimeAt(%q) = %s, want %s", c.in, s, c.want)
		}
	}
}

func TestParseTimeAt_Invalid(t *testing.T) {
	cases := []struct{ in, wantErr string }{
		{"yesterdy", "unrecognised time"},
		{"", "unrecognised time"},
		{"2027-06-30T25:00:00", "unrecognised time"},
		{"2026", "10 digits"},
		{"1700000000000", "milliseconds"},
		{"-1", "only accepted by --end"},
		{"forever", "only accepted by --end"},
		{"+99999999999d", "outside 1970-9999"},
		{"2027-06-30T16:00:00+08", "unrecognised time"},
		{"+0d", "invalid relative time"},
		{"+abc", "invalid relative time"},
		{"-d", "invalid relative time"},
		{"+5x", "unknown time unit"},
	}
	for _, c := range cases {
		_, err := common.ParseTimeAt(c.in, time.Now())
		if err == nil || !strings.Contains(err.Error(), c.wantErr) {
			t.Errorf("ParseTimeAt(%q) error = %v, want one containing %q", c.in, err, c.wantErr)
		}
	}
}

func TestAddTimeRange(t *testing.T) {
	flags := map[string]string{"since": "string", "until": "string"}
	cases := []struct {
		name      string
		values    map[string]string
		wantQuery map[string]any
		wantParam string
	}{
		{"neither", nil, map[string]any{}, ""},
		{"both normalised to RFC3339 UTC",
			map[string]string{"since": "2026-11-01", "until": "2026-11-02T08:00:00+08:00"},
			map[string]any{"placed_at_min": "2026-11-01T00:00:00Z", "placed_at_max": "2026-11-02T00:00:00Z"}, ""},
		{"unix seconds", map[string]string{"until": "1700000000"},
			map[string]any{"placed_at_max": "2023-11-14T22:13:20Z"}, ""},
		{"unparseable", map[string]string{"since": "yesterdy"}, nil, "--since"},
		{"since after until", map[string]string{"since": "2026-11-02", "until": "2026-11-01"}, nil, "--since"},
	}
	for _, c := range cases {
		t.Run(c.name, func(t *testing.T) {
			q := map[string]any{}
			err := common.AddTimeRange(q, shortcuttest.PlanInput(t, "search", flags, c.values), "placed_at")
			if c.wantParam != "" {
				var ee *output.ExitError
				if !errors.As(err, &ee) || ee.Code != output.ExitValidation || ee.Detail.Param != c.wantParam {
					t.Fatalf("want a validation error on %s, got %#v", c.wantParam, err)
				}
				return
			}
			if err != nil {
				t.Fatalf("unexpected error: %v", err)
			}
			if !reflect.DeepEqual(q, c.wantQuery) {
				t.Errorf("query = %v, want %v", q, c.wantQuery)
			}
		})
	}
}
