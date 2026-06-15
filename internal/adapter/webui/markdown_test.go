package webui

import (
	"strings"
	"testing"
)

func TestRenderMarkdown_Basic(t *testing.T) {
	got := string(RenderMarkdown("# Title\n\nsome **bold** text"))
	if !strings.Contains(got, "<h1") || !strings.Contains(got, "<strong>bold</strong>") {
		t.Errorf("markdown not rendered: %q", got)
	}
}

func TestRenderMarkdown_SanitizesScript(t *testing.T) {
	got := string(RenderMarkdown("hi\n\n<script>alert(1)</script>\n\n[x](javascript:alert(1))"))
	if strings.Contains(got, "<script>") || strings.Contains(got, "javascript:") {
		t.Errorf("unsafe content not sanitized: %q", got)
	}
}
