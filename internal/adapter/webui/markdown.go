package webui

import (
	"bytes"
	"html/template"

	"github.com/microcosm-cc/bluemonday"
	"github.com/yuin/goldmark"

	"github.com/serverkraken/flow/internal/domain"
)

var (
	md        = goldmark.New()
	ugcPolicy = bluemonday.UGCPolicy()
)

// RenderMarkdown converts user-authored Markdown to sanitised HTML safe for
// embedding in a template. The bluemonday UGC policy strips <script>,
// javascript: URLs, and other XSS vectors.
func RenderMarkdown(src string) template.HTML {
	if _, start := domain.ParseFrontmatter(src); start > 0 {
		src = src[start:]
	}
	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		return template.HTML(template.HTMLEscapeString(src))
	}
	clean := ugcPolicy.SanitizeBytes(buf.Bytes())
	return template.HTML(clean)
}
