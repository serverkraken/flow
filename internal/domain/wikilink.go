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
