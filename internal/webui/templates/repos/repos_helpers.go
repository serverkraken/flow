// Package repos renders the WebUI repos surface at `/repos` and
// `/repos/{escaped-canonical-key}/note`. All data resolution happens in
// the handler; templates only render formatted strings off a flat
// view-model.
//
// The list and detail are split into two routes (the mockup combines
// them; we follow the plan-level structure). The URL param on the
// detail route is the URL-escaped CanonicalKey — that keeps the URL
// idempotent across devices that have the same upstream cloned to
// different local paths.
package repos

import (
	"net/url"
	"strconv"
	"strings"
)

// FormatRepoTotal returns the page-header count pill. German uses
// "Repo" for both singular and plural to stay compact.
func FormatRepoTotal(total, withNotes int) string {
	var b strings.Builder
	b.WriteString(strconv.Itoa(total))
	if total == 1 {
		b.WriteString(" Repo")
	} else {
		b.WriteString(" Repos")
	}
	b.WriteString(" · ")
	b.WriteString(strconv.Itoa(withNotes))
	b.WriteString(" mit Notes")
	return b.String()
}

// NoteHref returns the detail URL for a given canonical key. The
// CanonicalKey may contain slashes ("git:gh.com/o/r") so we URL-escape
// the path segment.
func NoteHref(canonicalKey string) string {
	return "/repos/" + url.PathEscape(canonicalKey) + "/note"
}

// ShortHash returns the first 7 chars of a (likely-UUID) repo ID
// formatted as the "2f3a…91d" stub the mockup uses. For shorter IDs we
// fall back to the full string.
func ShortHash(id string) string {
	cleaned := strings.ReplaceAll(id, "-", "")
	if len(cleaned) < 10 {
		return cleaned
	}
	return cleaned[:4] + "…" + cleaned[len(cleaned)-3:]
}

// DisplayNameOr returns the repo's display-name, falling back to the
// canonical key when no display-name has been set yet. Keeps the list
// row from rendering an empty line.
func DisplayNameOr(displayName, canonicalKey string) string {
	if s := strings.TrimSpace(displayName); s != "" {
		return s
	}
	return canonicalKey
}
