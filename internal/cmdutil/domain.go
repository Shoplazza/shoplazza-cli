package cmdutil

import "strings"

// NormalizeStoreDomain returns a bare store host: it trims whitespace, strips
// a leading http(s):// scheme (case-insensitively), and drops trailing
// slashes. Callers prepend their own scheme, so without this "--store-domain
// https://x.com/" would yield a "https://https://x.com/" base URL.
func NormalizeStoreDomain(s string) string {
	s = strings.TrimSpace(s)
	switch lower := strings.ToLower(s); {
	case strings.HasPrefix(lower, "https://"):
		s = s[len("https://"):]
	case strings.HasPrefix(lower, "http://"):
		s = s[len("http://"):]
	}
	return strings.TrimRight(s, "/")
}
