package tui

import "regexp"

// weblinkSpan is one external link in a body line: a markdown `[text](url)` or
// a bare http(s) URL, with byte offsets into the line.
type weblinkSpan struct {
	Start, End int
	URL        string
	Display    string
}

var (
	// markdown link [text](http...): captured first so its inner URL is not
	// also matched as a bare URL.
	mdLinkRe = regexp.MustCompile(`\[([^\]]+)\]\((https?://[^\s)]+)\)`)
	// bare http(s) URL.
	bareURLRe = regexp.MustCompile(`https?://[^\s)]+`)
)

// findWeblinks returns external links in s, ordered by position, with markdown
// links taking precedence over the bare URLs they contain.
func findWeblinks(s string) []weblinkSpan {
	var spans []weblinkSpan
	taken := make([]bool, len(s))

	for _, m := range mdLinkRe.FindAllStringSubmatchIndex(s, -1) {
		spans = append(spans, weblinkSpan{
			Start: m[0], End: m[1], URL: s[m[4]:m[5]], Display: s[m[2]:m[3]],
		})
		for i := m[0]; i < m[1]; i++ {
			taken[i] = true
		}
	}
	for _, loc := range bareURLRe.FindAllStringIndex(s, -1) {
		if taken[loc[0]] {
			continue // inside a markdown link already captured
		}
		url := trimTrailingPunct(s[loc[0]:loc[1]])
		spans = append(spans, weblinkSpan{
			Start: loc[0], End: loc[0] + len(url), URL: url, Display: url,
		})
	}
	sortByStart(spans)
	return spans
}

func trimTrailingPunct(u string) string {
	for len(u) > 0 {
		switch u[len(u)-1] {
		case '.', ',', ')', ']', '}', '!', '?', ';', ':':
			u = u[:len(u)-1]
		default:
			return u
		}
	}
	return u
}

func sortByStart(s []weblinkSpan) {
	for i := 0; i < len(s); i++ {
		for j := i + 1; j < len(s); j++ {
			if s[j].Start < s[i].Start {
				s[i], s[j] = s[j], s[i]
			}
		}
	}
}
