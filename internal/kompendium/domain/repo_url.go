package domain

import "strings"

// CanonicalURL is a normalised git remote URL: lowercase, scheme-stripped,
// userinfo-stripped, ".git"-stripped, trailing-slash-stripped. Equal repos on
// different machines yield equal CanonicalURLs, so they map to the same
// notebook directory regardless of whether the user clones via SSH or HTTPS.
type CanonicalURL string

// NormalizeURL canonicalises a git remote URL.
//
// Examples:
//   - "git@github.com:Foo/Bar.git"         → "github.com/foo/bar"
//   - "https://github.com/Foo/Bar/"        → "github.com/foo/bar"
//   - "ssh://git@github.com/foo/bar.git"   → "github.com/foo/bar"
//   - "https://user:pass@host/foo/bar.git" → "host/foo/bar"
//
// Filesystem paths (used as the no-remote fallback by gitrepo.Detect) are
// preserved verbatim except for the trailing slash — POSIX paths are
// case-sensitive and macOS is case-preserving, so lowercasing them would
// produce a key that doesn't round-trip back to a real directory.
func NormalizeURL(raw string) CanonicalURL {
	s := strings.TrimSpace(raw)
	if isFilesystemPath(s) {
		return CanonicalURL(strings.TrimSuffix(s, "/"))
	}
	s = stripScheme(s)
	s = stripUserInfo(s)
	s = sshShortToHost(s)
	s = strings.TrimSuffix(s, ".git")
	s = strings.TrimSuffix(s, "/")
	s = strings.ToLower(s)
	return CanonicalURL(s)
}

// isFilesystemPath reports whether raw looks like a local path rather than
// a git URL. We treat absolute POSIX paths and "~"-rooted paths as such.
func isFilesystemPath(raw string) bool {
	return strings.HasPrefix(raw, "/") || strings.HasPrefix(raw, "~")
}

func stripScheme(s string) string {
	for _, p := range []string{"https://", "http://", "ssh://", "git://"} {
		if strings.HasPrefix(s, p) {
			return s[len(p):]
		}
	}
	return s
}

// stripUserInfo removes a leading "user@" or "user:pass@" segment, but only if
// it appears before the first path slash. Otherwise the "@" belongs to the
// path and must stay.
func stripUserInfo(s string) string {
	at := strings.Index(s, "@")
	if at == -1 {
		return s
	}
	slash := strings.Index(s, "/")
	if slash != -1 && at > slash {
		return s
	}
	return s[at+1:]
}

// sshShortToHost rewrites the SSH short form "host:path" → "host/path". It is
// safe to call after stripUserInfo, when no "user@" prefix remains. Non-SSH
// inputs (those with "/" before any ":") are returned unchanged.
func sshShortToHost(s string) string {
	colon := strings.Index(s, ":")
	if colon == -1 {
		return s
	}
	slash := strings.Index(s, "/")
	if slash != -1 && slash < colon {
		return s
	}
	return s[:colon] + "/" + s[colon+1:]
}

// String returns the canonical URL as a plain string.
func (c CanonicalURL) String() string {
	return string(c)
}

// Sanitize returns the canonical URL with path separators replaced so it can
// be used as a single filesystem directory name (e.g.
// "github.com/foo/bar" → "github.com_foo_bar"). The result is human-readable,
// not hashed.
func (c CanonicalURL) Sanitize() string {
	return strings.ReplaceAll(string(c), "/", "_")
}
