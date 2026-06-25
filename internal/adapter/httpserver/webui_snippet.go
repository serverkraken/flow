package httpserver

import (
	"html"
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// renderSnippet escapes the snippet then replaces highlight sentinels with
// <mark> tags. Escape-first prevents document content from injecting HTML.
func renderSnippet(s string) string {
	s = stripStraySentinels(s)
	esc := html.EscapeString(s)
	esc = strings.ReplaceAll(esc, domain.HighlightStart, "<mark>")
	esc = strings.ReplaceAll(esc, domain.HighlightEnd, "</mark>")
	return esc
}

func stripStraySentinels(s string) string {
	starts := strings.Count(s, domain.HighlightStart)
	ends := strings.Count(s, domain.HighlightEnd)
	if starts == 0 && ends == 0 {
		return s
	}
	if starts != ends {
		return domain.StripHighlightSentinels(s)
	}
	var out strings.Builder
	for {
		i := strings.Index(s, domain.HighlightStart)
		if i < 0 {
			out.WriteString(s)
			break
		}
		j := strings.Index(s[i:], domain.HighlightEnd)
		if j < 0 {
			out.WriteString(strings.ReplaceAll(strings.ReplaceAll(s, domain.HighlightStart, ""), domain.HighlightEnd, ""))
			break
		}
		j += i
		between := s[i+len(domain.HighlightStart) : j]
		if strings.Contains(between, domain.HighlightStart) {
			out.WriteString(s[:i])
			s = s[i+len(domain.HighlightStart):]
			continue
		}
		out.WriteString(s[:i])
		out.WriteString(domain.HighlightStart)
		out.WriteString(between)
		out.WriteString(domain.HighlightEnd)
		s = s[j+len(domain.HighlightEnd):]
	}
	return out.String()
}
