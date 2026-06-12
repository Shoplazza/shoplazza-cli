package build

import (
	"regexp"
	"strings"
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

var DefaultAuthBaseURL = "https://partners.shoplazza.com"

var DevPkgRoot = ""
