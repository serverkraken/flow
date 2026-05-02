package domain

import "regexp"

// Link is a wikilink reference extracted from a note body. Display is empty
// when the link uses the bare "[[id]]" form.
type Link struct {
	Target  string
	Display string
}

// wikiLinkRE matches "[[target]]" and "[[target|display]]" but rejects
// newlines inside the link, which are almost always bracket noise rather than
// a real wikilink.
var wikiLinkRE = regexp.MustCompile(`\[\[([^\]\n|]+)(?:\|([^\]\n]+))?\]\]`)

// ExtractLinks returns every wikilink in body, in document order.
func ExtractLinks(body []byte) []Link {
	matches := wikiLinkRE.FindAllSubmatch(body, -1)
	links := make([]Link, 0, len(matches))
	for _, m := range matches {
		l := Link{Target: string(m[1])}
		if len(m[2]) > 0 {
			l.Display = string(m[2])
		}
		links = append(links, l)
	}
	return links
}
