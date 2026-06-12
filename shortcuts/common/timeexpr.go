package common

import (
	"fmt"
	"strconv"
	"strings"
	"time"
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

// ParseTime converts a time expression to a Unix second integer.
//
// Supported formats:
//   - "now"               → current time
//   - "+30d" / "+2w" / "+12h" → relative offset (days/weeks/hours)
//   - "forever" / "-1"   → -1 (no expiry; valid for ends_at only)
//   - "2026-11-01"        → date at midnight UTC
//   - "2026-11-01T08:00:00" → RFC3339-ish datetime UTC
//   - plain integer string → parsed directly as Unix seconds
func ParseTime(s string) (int64, error) {
	s = strings.TrimSpace(s)
	switch s {
	case "now":
		return time.Now().Unix(), nil
	case "forever", "-1":
		return -1, nil
	}
	if strings.HasPrefix(s, "+") {
		return parseRelative(s[1:])
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n, nil
	}
	for _, layout := range []string{"2006-01-02T15:04:05", "2006-01-02"} {
		if t, err := time.ParseInLocation(layout, s, time.UTC); err == nil {
			return t.Unix(), nil
		}
	}
	return 0, fmt.Errorf("unrecognised time expression %q; use now, +30d, +12h, +2w, 2026-11-01, -1, or unix seconds", s)
}

func parseRelative(s string) (int64, error) {
	if len(s) < 2 {
		return 0, fmt.Errorf("invalid relative time %q", "+"+s)
	}
	unit := s[len(s)-1]
	n, err := strconv.ParseInt(s[:len(s)-1], 10, 64)
	if err != nil || n <= 0 {
		return 0, fmt.Errorf("invalid relative time %q", "+"+s)
	}
	now := time.Now()
	switch unit {
	case 'h':
		return now.Add(time.Duration(n) * time.Hour).Unix(), nil
	case 'd':
		return now.AddDate(0, 0, int(n)).Unix(), nil
	case 'w':
		return now.AddDate(0, 0, int(n)*7).Unix(), nil
	default:
		return 0, fmt.Errorf("unknown time unit %q in %q; use h (hours), d (days), w (weeks)", string(unit), "+"+s)
	}
}
