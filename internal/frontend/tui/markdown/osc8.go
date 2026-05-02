// OSC 8 hyperlink helpers. WrapURLs is a final post-process pass over
// bare http(s) URLs in already-rendered ANSI text; the goldmark node
// renderer (link / autolink / wikilink) emits OSC 8 sequences inline
// during walk.

package markdown

import (
	"regexp"
	"strings"
)

// urlRegex finds bare http(s) URLs in already-rendered ANSI text. The
// character class excludes whitespace, ANSI ESC, and the punctuation
// most likely to bracket a URL in prose so the match stops cleanly at a
// sentence boundary. Round parens are excluded too — that means URLs
// containing balanced parens (e.g. Wikipedia article slugs) get
// truncated at the first `(`. The simple form is good enough for the
// notes Soenne actually writes; a paren-balancing matcher is reserved
// for the day a real URL hits this corner.
var urlRegex = regexp.MustCompile(`https?://[^\s\x1b<>()"'\[\]]+`)

// urlTrailingTrim is the set of trailing characters that get stripped
// off a matched URL before it is wrapped. They typically belong to the
// surrounding sentence ("see https://example.com.") rather than the URL
// itself, so the trimmed punctuation stays *outside* the OSC 8 region.
const urlTrailingTrim = ".,;:!?)]>'\""

// WrapURLs wraps every http(s) URL in s in an OSC 8 hyperlink
// (`ESC ] 8 ;; URL BEL TEXT ESC ] 8 ;; BEL`) so terminals keep the link
// clickable across line wraps. lipgloss/cellbuf preserves OSC 8 across
// its hard-wrap by closing and re-opening the link around each
// newline, which is the whole point: a reflow upstream of this wrapper
// would otherwise drop the link state at every wrap boundary.
//
// Idempotent: if s already contains an OSC 8 sequence the input is
// returned unchanged. That keeps double-wrapping benign even when
// callers memoise the rendered output.
func WrapURLs(s string) string {
	if strings.Contains(s, "\x1b]8;") {
		return s
	}
	return urlRegex.ReplaceAllStringFunc(s, func(match string) string {
		url := strings.TrimRight(match, urlTrailingTrim)
		if url == "" {
			return match
		}
		wrapped := "\x1b]8;;" + url + "\x07" + url + "\x1b]8;;\x07"
		return wrapped + match[len(url):]
	})
}
