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
// notes that land in this notebook; a paren-balancing matcher is reserved
// for the day a real URL hits this corner.
var urlRegex = regexp.MustCompile(`https?://[^\s\x1b<>()"'\[\]]+`)

// urlTrailingTrim is the set of trailing characters that get stripped
// off a matched URL before it is wrapped. They typically belong to the
// surrounding sentence ("see https://example.com.") rather than the URL
// itself, so the trimmed punctuation stays *outside* the OSC 8 region.
const urlTrailingTrim = ".,;:!?)]>'\""

// WrapURLs wraps every bare http(s) URL in s in an OSC 8 hyperlink
// (`ESC ] 8 ;; URL BEL TEXT ESC ] 8 ;; BEL`) so terminals keep the link
// clickable across line wraps. lipgloss/cellbuf preserves OSC 8 across
// its hard-wrap by closing and re-opening the link around each
// newline, which is the whole point: a reflow upstream of this wrapper
// would otherwise drop the link state at every wrap boundary.
//
// Per-match idempotence: URLs already living inside an OSC 8 region
// (because the goldmark renderer emitted them as `[label](url)` or
// `[[wikilink]]`) are left untouched, but other bare URLs in the same
// document still get wrapped. The previous whole-document short-circuit
// silently dropped clickability for every bare URL once any single OSC 8
// sequence appeared upstream.
func WrapURLs(s string) string {
	matches := urlRegex.FindAllStringIndex(s, -1)
	if len(matches) == 0 {
		return s
	}
	regions := osc8Regions(s)
	var b strings.Builder
	b.Grow(len(s) + len(matches)*16)
	pos := 0
	for _, m := range matches {
		start, end := m[0], m[1]
		b.WriteString(s[pos:start])
		match := s[start:end]
		if insideRegion(start, regions) {
			b.WriteString(match)
			pos = end
			continue
		}
		url := strings.TrimRight(match, urlTrailingTrim)
		if url == "" {
			b.WriteString(match)
			pos = end
			continue
		}
		b.WriteString("\x1b]8;;")
		b.WriteString(url)
		b.WriteByte('\x07')
		b.WriteString(url)
		b.WriteString("\x1b]8;;\x07")
		b.WriteString(match[len(url):])
		pos = end
	}
	b.WriteString(s[pos:])
	return b.String()
}

// osc8Regions returns the byte-index spans [open .. closeEnd) covered by
// each well-formed OSC 8 hyperlink in s. The OSC 8 spec is
// `ESC ] 8 ; <params> ; <URL> BEL` for the open and `ESC ] 8 ; ; BEL`
// for the close — `<params>` may be empty (`;;`) or carry an `id=N`
// stamp (`;id=N;`) emitted by the renderer to keep multi-line wraps
// joined as one click target. The previous version only matched the
// no-params open shape (`\x1b]8;;`), missing every id-stamped open and
// causing WrapURLs to re-wrap URLs sitting inside the open header.
//
// Malformed input (open without close) is ignored — the leftover open
// marker simply doesn't produce a region.
func osc8Regions(s string) [][2]int {
	var regions [][2]int
	const prefix = "\x1b]8;"
	i := 0
	for {
		idx := strings.Index(s[i:], prefix)
		if idx < 0 {
			return regions
		}
		open := i + idx
		// Skip past the params (everything up to the second ';') and
		// past the URL (everything up to the BEL terminator).
		afterPrefix := open + len(prefix)
		paramsEnd := strings.IndexByte(s[afterPrefix:], ';')
		if paramsEnd < 0 {
			return regions
		}
		urlStart := afterPrefix + paramsEnd + 1
		bel := strings.IndexByte(s[urlStart:], '\x07')
		if bel < 0 {
			return regions
		}
		afterOpen := urlStart + bel + 1
		// Close form: a second `\x1b]8;;\x07` (empty params, empty URL,
		// BEL). Anything else here means malformed input we won't try
		// to recover from.
		closeIdx := strings.Index(s[afterOpen:], "\x1b]8;;\x07")
		if closeIdx < 0 {
			return regions
		}
		closeEnd := afterOpen + closeIdx + len("\x1b]8;;\x07")
		regions = append(regions, [2]int{open, closeEnd})
		i = closeEnd
	}
}

func insideRegion(pos int, regions [][2]int) bool {
	for _, r := range regions {
		if pos >= r[0] && pos < r[1] {
			return true
		}
	}
	return false
}
