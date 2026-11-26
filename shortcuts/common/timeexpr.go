package common

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/Shoplazza/shoplazza-cli/v2/internal/output"
)

// AutoName generates a CLI-scoped activity name: "cli_<tool>_<unix-sec>".
func AutoName(tool string) string {
	return fmt.Sprintf("cli_%s_%d", tool, time.Now().Unix())
}

// TruncateName truncates s to at most n runes.
func TruncateName(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n])
}

// TimeFormats lists the forms ParseTimeAt accepts; every time flag's help quotes it.
const TimeFormats = "now | today | yesterday | +Nh/-Nd/+Nw (relative to now) | 2026-11-01 | 2026-11-01 08:00[:00] | " +
	"with an offset (2026-11-01T08:00:00Z, 2026-11-01T08:00:00+08:00, 2026-11-01 08:00 +0800) | unix seconds; no offset = UTC"

var (
	zonedLayouts = layouts([]string{"Z07:00", "Z0700", " Z07:00", " Z0700"})
	utcLayouts   = append(layouts([]string{""}), "2006-01-02")
)

func layouts(zones []string) []string {
	var out []string
	for _, sep := range []string{"T", " "} {
		for _, clock := range []string{"15:04:05", "15:04"} {
			for _, zone := range zones {
				out = append(out, "2006-01-02"+sep+clock+zone)
			}
		}
	}
	return out
}

// ParseTimeAt resolves a time expression against now. Values without an
// offset, and the days named by today / yesterday, are read as UTC.
func ParseTimeAt(s string, now time.Time) (time.Time, error) {
	t, err := parseTime(strings.TrimSpace(s), now)
	if err != nil {
		return time.Time{}, err
	}
	if y := t.UTC().Year(); y < 1970 || y > 9999 {
		return time.Time{}, fmt.Errorf("%q resolves to year %d, outside 1970-9999", s, y)
	}
	return t, nil
}

func parseTime(s string, now time.Time) (time.Time, error) {
	switch strings.ToLower(s) {
	case "now":
		return now, nil
	case "today":
		return utcMidnight(now), nil
	case "yesterday":
		return utcMidnight(now).AddDate(0, 0, -1), nil
	case "forever":
		return time.Time{}, fmt.Errorf("%q means no expiry and is only accepted by --end", s)
	case "-1":
		return time.Time{}, fmt.Errorf("%q means no expiry and is only accepted by --end; for one day ago use -1d", s)
	}
	if strings.HasPrefix(s, "+") || strings.HasPrefix(s, "-") {
		return parseRelative(s, now)
	}
	if isDigits(s) {
		switch {
		case s == "0" || len(s) == 9 || len(s) == 10:
			n, _ := strconv.ParseInt(s, 10, 64)
			return time.Unix(n, 0).UTC(), nil
		case len(s) == 13:
			return time.Time{}, fmt.Errorf("%q looks like unix milliseconds; pass unix seconds (10 digits)", s)
		default:
			return time.Time{}, fmt.Errorf("unrecognised time %q; write a date as 2026-11-01, and unix seconds as 9-10 digits (or 0)", s)
		}
	}
	upper := strings.ToUpper(s)
	for _, layout := range zonedLayouts {
		if t, err := time.Parse(layout, upper); err == nil {
			return t.UTC(), nil
		}
	}
	for _, layout := range utcLayouts {
		if t, err := time.ParseInLocation(layout, upper, time.UTC); err == nil {
			return t, nil
		}
	}
	return time.Time{}, fmt.Errorf("unrecognised time %q; accepted: %s", s, TimeFormats)
}

// TimeFlag parses the named time flag with ParseTimeAt; set is false when the
// flag was not passed. A bad value is a validation error naming the flag.
func TimeFlag(in PlanInput, name string) (t time.Time, set bool, err error) {
	if !in.Flags.Changed(name) {
		return time.Time{}, false, nil
	}
	t, err = ParseTimeAt(in.Flags.GetString(name), time.Now())
	if err != nil {
		return time.Time{}, true, invalidTime(name, err)
	}
	return t, true, nil
}

// AddTimeRange writes --since / --until into q as RFC3339 UTC under
// <field>_min / <field>_max, rejecting a window whose start is after its end.
func AddTimeRange(q map[string]any, in PlanInput, field string) error {
	return AddTimeRangeAs(q, in, field+"_min", field+"_max")
}

// AddTimeRangeAs is AddTimeRange for APIs whose bound keys do not follow <field>_min / <field>_max.
func AddTimeRangeAs(q map[string]any, in PlanInput, minKey, maxKey string) error {
	since, hasSince, err := TimeFlag(in, "since")
	if err != nil {
		return err
	}
	until, hasUntil, err := TimeFlag(in, "until")
	if err != nil {
		return err
	}
	if hasSince && hasUntil && since.After(until) {
		return output.ErrValidation("--since (%s) is after --until (%s)", rfc3339(since), rfc3339(until)).
			WithParam("--since").
			WithSubtype(output.SubtypeInvalidFlagValue)
	}
	if hasSince {
		q[minKey] = rfc3339(since)
	}
	if hasUntil {
		q[maxKey] = rfc3339(until)
	}
	return nil
}

func invalidTime(name string, err error) *output.ExitError {
	return output.ErrValidation("invalid --%s: %v", name, err).
		WithParam("--" + name).
		WithSubtype(output.SubtypeInvalidFlagValue).
		WithHint("accepted: " + TimeFormats)
}

func rfc3339(t time.Time) string {
	return t.UTC().Format(time.RFC3339)
}

func utcMidnight(t time.Time) time.Time {
	y, m, d := t.UTC().Date()
	return time.Date(y, m, d, 0, 0, 0, 0, time.UTC)
}

func isDigits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}

func parseRelative(s string, now time.Time) (time.Time, error) {
	sign, body := s[:1], s[1:]
	if len(body) < 2 {
		return time.Time{}, fmt.Errorf("invalid relative time %q; use e.g. +12h, -7d, +2w", s)
	}
	n, err := strconv.Atoi(body[:len(body)-1])
	if err != nil || n <= 0 {
		return time.Time{}, fmt.Errorf("invalid relative time %q; use e.g. +12h, -7d, +2w", s)
	}
	if sign == "-" {
		n = -n
	}
	switch body[len(body)-1] {
	case 'h':
		return now.Add(time.Duration(n) * time.Hour), nil
	case 'd':
		return now.AddDate(0, 0, n), nil
	case 'w':
		return now.AddDate(0, 0, n*7), nil
	default:
		return time.Time{}, fmt.Errorf("unknown time unit in %q; use h (hours), d (days), w (weeks)", s)
	}
}
