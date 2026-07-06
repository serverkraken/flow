package webui

import (
	"strings"

	"github.com/serverkraken/flow/internal/domain"
)

// ReadingTime estimates the reading time of a document body in minutes, at
// 220 words/minute, rounded up, with a floor of 1. Frontmatter is stripped
// first so its YAML keys never inflate the word count. i18n-free: the
// caller/templ layer supplies the "N min" label text.
func ReadingTime(body string) int {
	if _, start := domain.ParseFrontmatter(body); start > 0 {
		body = body[start:]
	}
	words := len(strings.Fields(body))
	mins := (words + 219) / 220
	if mins < 1 {
		mins = 1
	}
	return mins
}
