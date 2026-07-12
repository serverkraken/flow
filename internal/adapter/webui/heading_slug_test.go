package webui

import (
	"context"
	"strings"
	"testing"
)

func TestHeadingSlugify_GitHubStyle(t *testing.T) {
	cases := map[string]string{
		"Mein Über-Titel": "mein-ueber-titel",
		"Hello World":     "hello-world",
		"  spaces  ":      "spaces",
		"":                "",
	}
	for in, want := range cases {
		if got := headingSlugify(in); got != want {
			t.Errorf("headingSlugify(%q) = %q, want %q", in, got, want)
		}
	}
}

func TestRenderDocument_HeadingGetsSlugID(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "## Mein Über-Titel\n", resolveNone, nil)))
	if !strings.Contains(out, `id="mein-ueber-titel"`) {
		t.Fatalf("expected heading id=mein-ueber-titel, got: %s", out)
	}
}

func TestRenderDocument_DuplicateHeadingsGetSuffixedIDs(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "## Setup\n\ntext\n\n## Setup\n", resolveNone, nil)))
	if !strings.Contains(out, `id="setup"`) {
		t.Fatalf("expected first heading id=setup, got: %s", out)
	}
	if !strings.Contains(out, `id="setup-1"`) {
		t.Fatalf("expected second (duplicate) heading id=setup-1, got: %s", out)
	}
}

func TestRenderDocument_HeadingIDSurvivesSanitizer(t *testing.T) {
	// The sanitizer is applied inside RenderDocument itself, so a heading id
	// still present in the returned HTML is the roundtrip proof: bluemonday
	// already ran and did not strip it.
	out := string(mustHTML(RenderDocument(context.Background(), "## Roundtrip Check\n", resolveNone, nil)))
	if !strings.Contains(out, `<h2 id="roundtrip-check"`) {
		t.Fatalf("expected <h2 id=...> to survive SanitizeBytes, got: %s", out)
	}
}

func TestRenderDocument_HeadingHasHoverParagraphAnchor(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "## Mein Über-Titel\n", resolveNone, nil)))
	if !strings.Contains(out, `<a class="head-anchor" href="#mein-ueber-titel"`) {
		t.Fatalf("expected head-anchor <a> for the heading, got: %s", out)
	}
	if !strings.Contains(out, ">¶</a>") {
		t.Fatalf("expected the anchor's text to be the monospace ¶ glyph, got: %s", out)
	}
	// The anchor must be inside the heading (last child), not a sibling.
	if !strings.Contains(out, `Mein Über-Titel<a class="head-anchor"`) {
		t.Fatalf("expected head-anchor to be the heading's last child, got: %s", out)
	}
}

func TestRenderDocument_NoHeadingsNoAnchors(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "just a paragraph, no headings\n", resolveNone, nil)))
	if strings.Contains(out, "head-anchor") {
		t.Fatalf("expected no head-anchor when the document has no headings, got: %s", out)
	}
}
