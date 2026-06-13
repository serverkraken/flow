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

	"github.com/a-h/templ"
)

// NoteEditHref returns the URL of the repo-note edit form for a given
// canonical key. The CanonicalKey may contain slashes ("git:gh.com/o/r")
// so we URL-escape the path segment.
func NoteEditHref(canonicalKey string) string {
	return "/repos/" + url.PathEscape(canonicalKey) + "/note/edit"
}

// noteHrefSafe wraps NoteHref in a templ.SafeURL so the templ surface
// can use the helper inside href attributes without further escaping.
func noteHrefSafe(canonicalKey string) templ.SafeURL {
	return templ.SafeURL(NoteHref(canonicalKey))
}

// noteEditFormAction returns the URL the edit form posts to. Same path
// as the view, with the HTML form using POST + _method=PUT for the
// no-JS fallback (HTMX upgrades the request to a true PUT when available).
func noteEditFormAction(canonicalKey string) templ.SafeURL {
	return templ.SafeURL("/repos/" + url.PathEscape(canonicalKey) + "/note")
}

// noteEditHrefSafe wraps NoteEditHref for templ href attributes.
func noteEditHrefSafe(canonicalKey string) templ.SafeURL {
	return templ.SafeURL(NoteEditHref(canonicalKey))
}

// formatVersion converts an int64 OCC version to its decimal string
// for hidden form fields. Kept inline (no fmt import) so the templ
// package stays lean.
func formatVersion(v int64) string {
	return strconv.FormatInt(v, 10)
}

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
