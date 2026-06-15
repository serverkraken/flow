package webui

import (
	"strings"
	"testing"
)

func TestRenderDocument_Wikilinks(t *testing.T) {
	resolve := func(target string) (string, string, bool) {
		if target == "arch" {
			return "/docs/d-arch", "Architecture", true
		}
		return "", "", false
	}
	html := string(RenderDocument("see [[arch]] and [[ghost]] and [[arch|the arch]]", resolve))

	if !strings.Contains(html, `href="/docs/d-arch"`) {
		t.Errorf("valid wikilink should link to /docs/d-arch:\n%s", html)
	}
	if !strings.Contains(html, "Architecture") {
		t.Errorf("valid wikilink should use resolved title as display:\n%s", html)
	}
	if !strings.Contains(html, "the arch") {
		t.Errorf("explicit display should win:\n%s", html)
	}
	if !strings.Contains(html, "wikilink-broken") {
		t.Errorf("ghost should render broken:\n%s", html)
	}
}

func TestRenderDocument_StillSanitises(t *testing.T) {
	html := string(RenderDocument("<script>alert(1)</script> [[x]]", func(string) (string, string, bool) {
		return "", "", false
	}))
	if strings.Contains(html, "<script>") {
		t.Errorf("script must be stripped:\n%s", html)
	}
}

func TestRenderDocument_RealLinksNative(t *testing.T) {
	html := string(RenderDocument("[site](https://example.com)", func(string) (string, string, bool) {
		return "", "", false
	}))
	if !strings.Contains(html, `href="https://example.com"`) {
		t.Errorf("real markdown links should render natively:\n%s", html)
	}
}
