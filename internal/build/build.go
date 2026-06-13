package build

import (
	"regexp"
	"strings"
	"time"
)

var (
	Version = "dev"
	Date    = "unknown"
)

var describeSuffix = regexp.MustCompile(`-\d+-g[0-9a-f]+$`)

func DisplayVersion() string {
	v := strings.TrimSuffix(Version, "-dirty")
	if loc := describeSuffix.FindStringIndex(v); loc != nil {
		v = v[:loc[0]]
	}
	return v
}

// DisplayDate returns the build date truncated to day precision (YYYY-MM-DD).
// Falls back to the raw Date when it isn't a parseable RFC3339 timestamp
// (e.g. the "unknown" default or an empty value).
func DisplayDate() string {
	if t, err := time.Parse(time.RFC3339, Date); err == nil {
		return t.Format("2006-01-02")
	}
	return Date
}

var DefaultAuthBaseURL = "https://partners.shoplazza.com"

var DevPkgRoot = ""
