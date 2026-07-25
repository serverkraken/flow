package main

import (
	"fmt"
	"sort"
	"strings"
	"unicode"

	"github.com/serverkraken/flow/internal/domain"
)

// formatDocLine renders one document's metadata as a single line.
func formatDocLine(d domain.Document) string {
	line := fmt.Sprintf("- [%s] %s · %s · %s", d.ID, d.Title, d.Path, d.Type)
	if len(d.Tags) > 0 {
		line += " · tags: " + strings.Join(d.Tags, ", ")
	}
	return line
}

// formatDocList renders a metadata list with a scope-describing header.
func formatDocList(docs []domain.Document, sc scope) string {
	if len(docs) == 0 {
		return "No documents " + sc.label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) %s:\n", len(docs), sc.label)
	for _, d := range docs {
		b.WriteString(formatDocLine(d))
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatSearchHits renders search hits (metadata line + indented snippet).
func formatSearchHits(hits []domain.SearchHit, query string, sc scope) string {
	if len(hits) == 0 {
		return fmt.Sprintf("No matches for %q %s.", query, sc.label)
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d match(es) for %q %s:\n", len(hits), query, sc.label)
	for _, h := range hits {
		b.WriteString(formatDocLine(h.Document))
		if s := conciseSearchSnippet(h, query); s != "" {
			fmt.Fprintf(&b, "\n    %s", s)
		}
		b.WriteByte('\n')
	}
	return strings.TrimRight(b.String(), "\n")
}

const maxMCPSearchSnippetRunes = 320

func conciseSearchSnippet(hit domain.SearchHit, query string) string {
	snippet := domain.StripHighlightSentinels(strings.TrimSpace(hit.Snippet))
	body := domain.StripHighlightSentinels(hit.Body)
	term := bestSearchTerm(query, snippet)
	source := snippet
	if term == "" {
		if bodyTerm := bestSearchTerm(query, body); bodyTerm != "" {
			source, term = body, bodyTerm
		}
	}
	if strings.TrimSpace(source) == "" {
		return ""
	}
	return centeredSnippet(source, term, maxMCPSearchSnippetRunes)
}

func bestSearchTerm(query, text string) string {
	lower := strings.ToLower(text)
	candidates := []string{strings.TrimSpace(query)}
	for _, field := range strings.Fields(query) {
		field = strings.TrimFunc(field, func(r rune) bool { return !unicode.IsLetter(r) && !unicode.IsNumber(r) })
		if field != "" {
			candidates = append(candidates, field)
		}
	}
	best := ""
	for _, candidate := range candidates {
		if len([]rune(candidate)) <= len([]rune(best)) {
			continue
		}
		if strings.Contains(lower, strings.ToLower(candidate)) {
			best = candidate
		}
	}
	return best
}

func centeredSnippet(source, term string, limit int) string {
	runes := []rune(source)
	if len(runes) <= limit {
		return strings.Join(strings.Fields(source), " ")
	}
	center := 0
	if term != "" {
		if runeAt := equalFoldRuneIndex(runes, []rune(term)); runeAt >= 0 {
			center = runeAt + len([]rune(term))/2
		}
	}
	start := center - limit/2
	if start < 0 {
		start = 0
	}
	end := start + limit
	if end > len(runes) {
		end = len(runes)
		start = end - limit
	}
	out := strings.Join(strings.Fields(string(runes[start:end])), " ")
	if start > 0 {
		out = "…" + out
	}
	if end < len(runes) {
		out += "…"
	}
	return out
}

func equalFoldRuneIndex(text, term []rune) int {
	if len(term) == 0 || len(term) > len(text) {
		return -1
	}
	for i := 0; i+len(term) <= len(text); i++ {
		if strings.EqualFold(string(text[i:i+len(term)]), string(term)) {
			return i
		}
	}
	return -1
}

// formatDoc renders a full document for flow_get_doc.
func formatDoc(d domain.Document, projectName string) string {
	proj := "—"
	if projectName != "" {
		proj = projectName
	}
	tags := "—"
	if len(d.Tags) > 0 {
		tags = strings.Join(d.Tags, ", ")
	}
	meta := fmt.Sprintf("%s · %s · project: %s · tags: %s", d.Path, d.Type, proj, tags)
	if d.Role != nil && *d.Role != "" {
		meta += " · role: " + *d.Role
	}
	return fmt.Sprintf("%s\n%s\nid: %s\n\n%s", d.Title, meta, d.ID, d.Body)
}

// formatTags renders tag counts, highest first.
func formatTags(tags []domain.TagCount, sc scope) string {
	if len(tags) == 0 {
		return "No tags " + sc.label + "."
	}
	sorted := make([]domain.TagCount, len(tags))
	copy(sorted, tags)
	sort.SliceStable(sorted, func(i, j int) bool { return sorted[i].Count > sorted[j].Count })
	var b strings.Builder
	fmt.Fprintf(&b, "Tags %s:\n", sc.label)
	for _, t := range sorted {
		fmt.Fprintf(&b, "- %s (%d)\n", t.Tag, t.Count)
	}
	return strings.TrimRight(b.String(), "\n")
}

// formatBacklinks renders inbound wikilink references to a document.
func formatBacklinks(refs []domain.BacklinkRef, label string) string {
	if len(refs) == 0 {
		return "No documents link to " + label + "."
	}
	var b strings.Builder
	fmt.Fprintf(&b, "%d document(s) link to %s:\n", len(refs), label)
	for _, r := range refs {
		fmt.Fprintf(&b, "- [%s] %s · %s · %s\n", r.ID, r.Title, r.Path, r.Type)
	}
	return strings.TrimRight(b.String(), "\n")
}
