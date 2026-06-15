package domain

// WikilinkSpan is one `[[target]]` / `[[target|display]]` occurrence in a
// body, with byte offsets so a renderer can slice the surrounding text.
type WikilinkSpan struct {
	Start, End int // byte offsets into the source; [Start,End) covers the whole `[[...]]`
	Target     string
	Display    string // explicit display half; "" when no pipe
}

// FindWikilinks scans s for wikilinks. A candidate aborts at a newline (a
// wikilink never spans a line break) and an empty target is not a match, so
// other `[...]` constructs fall through untouched.
func FindWikilinks(s string) []WikilinkSpan {
	var out []WikilinkSpan
	for i := 0; i+1 < len(s); i++ {
		if s[i] != '[' || s[i+1] != '[' {
			continue
		}
		end := -1
		for j := i + 2; j+1 < len(s); j++ {
			if s[j] == '\n' {
				break
			}
			if s[j] == ']' && s[j+1] == ']' {
				end = j
				break
			}
		}
		if end < 0 {
			continue
		}
		target, display := splitWikilinkInner(s[i+2 : end])
		if target == "" {
			continue
		}
		out = append(out, WikilinkSpan{Start: i, End: end + 2, Target: target, Display: display})
		i = end + 1 // resume after `]]` (loop's i++ lands past it)
	}
	return out
}

// splitWikilinkInner splits "target|display". Display is empty without a pipe.
// A newline or stray ']' before the first '|' aborts the match (empty target);
// text after the pipe is returned verbatim as the display.
func splitWikilinkInner(s string) (target, display string) {
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '|':
			return s[:i], s[i+1:]
		case '\n', ']':
			return "", ""
		}
	}
	return s, ""
}

// BacklinkRef is a lightweight reference to a document that links to another,
// surfaced in "Referenced by". Shared by the backlinks use case, REST, and the
// API client.
type BacklinkRef struct {
	ID    string       `json:"id"`
	Path  string       `json:"path"`
	Title string       `json:"title"`
	Type  DocumentType `json:"type"`
}

// sameScope reports whether two documents share a wikilink resolution scope:
// the same project, or both free/owner-level (nil ProjectID).
func sameScope(a, b *string) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return *a == *b
}

// ResolveWikilink resolves target against the owner's document set all, from the
// perspective of src. Scope-isolated: a same-scope match wins; else a free
// (ProjectID == nil) match; else broken. A foreign-project match never
// resolves, even when owner-wide unique.
func ResolveWikilink(src Document, target string, all []Document) (Document, bool) {
	var free *Document
	for i := range all {
		d := all[i]
		if d.Path != target {
			continue
		}
		if sameScope(src.ProjectID, d.ProjectID) {
			return d, true
		}
		if d.ProjectID == nil && free == nil {
			free = &all[i]
		}
	}
	if free != nil {
		return *free, true
	}
	return Document{}, false
}

// WikilinkTargets returns the ordered, de-duplicated target paths in body,
// for the link index. Returns nil when there are none.
func WikilinkTargets(body string) []string {
	spans := FindWikilinks(body)
	if len(spans) == 0 {
		return nil
	}
	seen := make(map[string]bool, len(spans))
	var out []string
	for _, sp := range spans {
		if !seen[sp.Target] {
			seen[sp.Target] = true
			out = append(out, sp.Target)
		}
	}
	return out
}
