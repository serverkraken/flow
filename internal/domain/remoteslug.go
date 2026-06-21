package domain

import (
	"regexp"
	"strings"
)

// scpLike matches git's scp form: [user@]host:path
var scpLike = regexp.MustCompile(`^(?:[^@/]+@)?([^/:]+):(.+)$`)

// NormalizeRemoteSlug turns any git remote URL form into a stable, lowercased
// "host/path" slug, or ok=false when it can't be parsed.
func NormalizeRemoteSlug(raw string) (string, bool) {
	s := strings.TrimSpace(raw)
	if s == "" {
		return "", false
	}
	var host, path string
	switch {
	case strings.Contains(s, "://"): // scheme form
		rest := s[strings.Index(s, "://")+3:]
		if at := strings.LastIndex(rest, "@"); at >= 0 { // strip credentials
			rest = rest[at+1:]
		}
		slash := strings.Index(rest, "/")
		if slash < 0 {
			return "", false
		}
		host, path = rest[:slash], rest[slash+1:]
	case scpLike.MatchString(s): // git@host:path
		m := scpLike.FindStringSubmatch(s)
		host, path = m[1], m[2]
	default:
		return "", false
	}
	if i := strings.Index(host, ":"); i >= 0 { // strip port
		host = host[:i]
	}
	path = strings.TrimSuffix(strings.TrimSuffix(strings.Trim(path, "/"), ".git"), "/")
	if host == "" || path == "" {
		return "", false
	}
	return strings.ToLower(host + "/" + path), true
}
