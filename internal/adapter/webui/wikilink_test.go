package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/i18n"
)

// TestWikiLinkNodeDump covers wikiLinkNode.Dump (0% coverage).
// Dump is goldmark's AST debug helper and is only called by ast.Dump();
// verify it does not panic with zero-value inputs.
func TestWikiLinkNodeDump(t *testing.T) {
	w := &wikiLinkNode{Target: "note-abc", Display: "Note ABC"}
	w.Dump([]byte{}, 0)
}

// --- Task L6-3: ![[slug]] artifact embeds --------------------------------

// TestArtifactEmbed_CoreImageNotStolen is the KRITISCH parser-priority test:
// a normal Markdown image `![alt](url)` must stay goldmark's core Image
// node — never our artifactEmbedNode — even though wikiLinkParser triggers
// on both '!' and '['. If the parser wrongly claimed this, the output would
// carry our figure/broken-embed markup instead of a plain <img>.
func TestArtifactEmbed_CoreImageNotStolen(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "![alt](https://x/y.png)\n", resolveNone, nil)))
	if !strings.Contains(out, "<img") {
		t.Fatalf("expected a core <img> element, got: %s", out)
	}
	if strings.Contains(out, `class="figure"`) || strings.Contains(out, "wikilink-broken") {
		t.Fatalf("a Markdown image must never be treated as an artifact embed: %s", out)
	}
}

// TestArtifactEmbed_ResolvedImage covers a resolved ![[slug]] embed that
// IsImage: it becomes a numbered <figure> with a "?v={ref}" versioned src.
func TestArtifactEmbed_ResolvedImage(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	resolve := func(slug string) (ArtifactRef, bool) {
		if slug != "bild" {
			return ArtifactRef{}, false
		}
		return ArtifactRef{
			Href: "/nodes/node-1/artifacts/bild", Ref: "abcdef123456",
			Name: "bild.png", Mime: "image/png", IsImage: true, Width: 10, Height: 8,
		}, true
	}
	out := string(mustHTML(RenderDocument(ctx, "![[bild]]\n", resolveNone, resolve)))
	for _, want := range []string{
		`class="figure"`,
		`src="/nodes/node-1/artifacts/bild?v=abcdef123456"`,
		"Abb. 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("resolved image embed misses %q:\n%s", want, out)
		}
	}
}

// TestArtifactEmbed_ResolvedFile covers a resolved ![[slug]] embed whose
// artifact is NOT an image: it becomes a downloadable .filechip instead of
// an <img>, using the bare Href (no "?v=") per Spec §5.
func TestArtifactEmbed_ResolvedFile(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	resolve := func(slug string) (ArtifactRef, bool) {
		if slug != "spec" {
			return ArtifactRef{}, false
		}
		return ArtifactRef{
			Href: "/nodes/node-1/artifacts/spec", Ref: "111111111111",
			Name: "spec.pdf", Mime: "application/pdf", SizeStr: "2.0 KB",
		}, true
	}
	out := string(mustHTML(RenderDocument(ctx, "![[spec]]\n", resolveNone, resolve)))
	for _, want := range []string{
		`class="filechip"`,
		`href="/nodes/node-1/artifacts/spec"`,
		"download",
		"spec.pdf",
		"2.0 KB",
		"Abb. 1",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("resolved file embed misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "?v=") {
		t.Fatalf("file chip must use the bare href (no ?v=, no-cache download): %s", out)
	}
}

// TestArtifactEmbed_UnresolvedSlugBrokenSpan covers the "not found / doc
// unbound" state: an unresolved ![[slug]] never breaks the render, it just
// becomes the same visible marker as an unresolved [[wikilink]].
func TestArtifactEmbed_UnresolvedSlugBrokenSpan(t *testing.T) {
	out := string(mustHTML(RenderDocument(context.Background(), "![[missing]]\n", resolveNone, nil)))
	if !strings.Contains(out, `class="wikilink-broken"`) || !strings.Contains(out, "missing") {
		t.Fatalf("expected a broken-wikilink span for the unresolved embed: %s", out)
	}
}

// TestArtifactEmbed_MalformedFormsFallBackCleanly is the KRITISCH parser
// regression test: a truncated "![[" (no closing "]]"), a too-short "![[x]",
// and a lone '!' that isn't the start of "![[" must all fall through to
// goldmark's core parsers cleanly — no panic, no artifactEmbedNode.
func TestArtifactEmbed_MalformedFormsFallBackCleanly(t *testing.T) {
	for _, src := range []string{
		"See ![[unterminated for details.\n",
		"See ![[x] for details.\n",
		"Wow! that worked.\n",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), src, resolveNone, nil)))
		if strings.Contains(out, `class="figure"`) {
			t.Fatalf("malformed form must not become an artifact embed, src=%q out=%s", src, out)
		}
	}
}

// TestFigureTransformer_MixedMermaidArtifactNumbering is the KRITISCH shared-
// counter test (Spec: figures share ONE numbering pass with Mermaid, in
// document order): Mermaid, resolved image, an UNRESOLVED embed that must
// NOT consume a number, then another Mermaid — the unresolved embed must
// not create a numbering gap ("Abb. 1..3", never "Abb. 4" anywhere).
func TestFigureTransformer_MixedMermaidArtifactNumbering(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	resolve := func(slug string) (ArtifactRef, bool) {
		if slug != "bild" {
			return ArtifactRef{}, false
		}
		return ArtifactRef{Href: "/nodes/n1/artifacts/bild", Ref: "aaaaaaaaaaaa", Name: "bild.png", IsImage: true}, true
	}
	src := strings.Join([]string{
		"```mermaid",
		"A-->B",
		"```",
		"",
		"![[bild]]",
		"",
		"![[missing]]",
		"",
		"```mermaid",
		"C-->D",
		"```",
	}, "\n")
	out := string(mustHTML(RenderDocument(ctx, src, resolveNone, resolve)))
	for _, want := range []string{"Abb. 1", "Abb. 2", "Abb. 3"} {
		if !strings.Contains(out, want) {
			t.Fatalf("missing %q in mixed Mermaid/artifact numbering:\n%s", want, out)
		}
	}
	if strings.Contains(out, "Abb. 4") {
		t.Fatalf("unresolved embed must not consume a figure number:\n%s", out)
	}
	if !strings.Contains(out, "wikilink-broken") {
		t.Fatalf("unresolved embed must still render as a broken span:\n%s", out)
	}
}

// --- Sanitizer boundary: only the artifact serve route survives as <img src>

// TestSafeImageRenderer_RejectsNonArtifactSrc is the KRITISCH sanitizer
// test: bluemonday's UGCPolicy alone allows ANY img src (AllowImages() adds
// a nil-Matching, i.e. match-everything, policy entry, and attribute
// policies for the same element+attr are OR-combined — see wikilink.go's
// safeImageHTMLRenderer doc comment for the verified mechanism), so an
// external host, a data: URI, and a protocol-relative URL must all be
// rejected by the renderer override itself, not by bluemonday.
func TestSafeImageRenderer_RejectsNonArtifactSrc(t *testing.T) {
	cases := []struct{ name, url string }{
		{"external host", "https://evil.example/x.png"},
		{"data URI", "data:image/png;base64,AAAA"},
		{"protocol-relative", "//evil.example/x.png"},
	}
	for _, c := range cases {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+c.url+")\n", resolveNone, nil)))
		if strings.Contains(out, c.url) {
			t.Fatalf("%s: malicious src leaked into rendered output: %s", c.name, out)
		}
		if !strings.Contains(out, "<img") {
			t.Fatalf("%s: expected the <img> element itself to survive (just without a usable src): %s", c.name, out)
		}
		// bluemonday drops an empty `src=""` attribute entirely rather than
		// keeping it — assert no dangerous scheme survived under any src
		// form, rather than pinning the exact `src=""` shape.
		if strings.Contains(out, `src="http`) || strings.Contains(out, `src="data:`) || strings.Contains(out, `src="//`) {
			t.Fatalf("%s: src attribute retained a dangerous scheme: %s", c.name, out)
		}
	}
}

// TestSafeImageRenderer_AllowsArtifactServeRoute is the positive control for
// the negative tests above: the one src shape that IS allowed to survive
// (node-form serve route, both bare and versioned).
func TestSafeImageRenderer_AllowsArtifactServeRoute(t *testing.T) {
	for _, url := range []string{
		"/nodes/n1/artifacts/bild",
		"/nodes/n1/artifacts/bild?v=abcdef123456",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, `src="`+url+`"`) {
			t.Fatalf("expected the node-form artifact serve-route src to survive untouched: %s", out)
		}
	}
}

// TestSafeImageRenderer_AllowsFreeArtifactServeRoute is the free-artifact-read-path
// (Task 2) positive control: the NEW `/artefakte/{slug}` serve form must
// survive the sanitizer identically to the node form above, both bare and
// versioned.
func TestSafeImageRenderer_AllowsFreeArtifactServeRoute(t *testing.T) {
	for _, url := range []string{
		"/artefakte/bild",
		"/artefakte/bild?v=abcdef123456",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, `src="`+url+`"`) {
			t.Fatalf("expected the free-artifact serve-route src to survive untouched: %s", out)
		}
	}
}

// --- fr-doc-lightbox: zoombare Bilder ------------------------------------

// TestZoomable_ArtifactEmbedImage pins that a resolved ![[slug]] image embed
// carries the class the lightbox selects on. The file chip (non-image
// artifact) must NOT get it — there is nothing to enlarge.
func TestZoomable_ArtifactEmbedImage(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	resolve := func(slug string) (ArtifactRef, bool) {
		switch slug {
		case "bild":
			return ArtifactRef{
				Href: "/nodes/node-1/artifacts/bild", Ref: "abcdef123456",
				Name: "bild.png", Mime: "image/png", IsImage: true,
			}, true
		case "datei":
			return ArtifactRef{
				Href: "/nodes/node-1/artifacts/datei", Ref: "abcdef123456",
				Name: "datei.pdf", Mime: "application/pdf", SizeStr: "1.0 KB",
			}, true
		}
		return ArtifactRef{}, false
	}

	img := string(mustHTML(RenderDocument(ctx, "![[bild]]\n", resolveNone, resolve)))
	if !strings.Contains(img, `class="zoomable"`) {
		t.Fatalf("resolved image embed must be zoomable:\n%s", img)
	}

	chip := string(mustHTML(RenderDocument(ctx, "![[datei]]\n", resolveNone, resolve)))
	if strings.Contains(chip, "zoomable") {
		t.Fatalf("a non-image artifact chip must not be zoomable:\n%s", chip)
	}
}

// TestZoomable_CoreImageWithValidSrc covers the second image syntax: a core
// ![alt](url) pointing at an allowed artifact serve route earns the class,
// in both the node-scoped and the free-library form.
func TestZoomable_CoreImageWithValidSrc(t *testing.T) {
	for _, url := range []string{
		"/nodes/n1/artifacts/bild",
		"/nodes/n1/artifacts/bild?v=abcdef123456",
		"/artefakte/bild",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, `class="zoomable"`) {
			t.Fatalf("core image with allowed src %q must be zoomable:\n%s", url, out)
		}
	}
}

// TestZoomable_BlockedSrcHasNoClass is the load-bearing negative: a rejected
// destination leaves safeImageHTMLRenderer emitting an <img> with an empty
// src (bluemonday then drops the attribute entirely). Marking THAT image
// zoomable would open an empty lightbox on click, so the class must be tied
// to a usable src — not emitted unconditionally.
func TestZoomable_BlockedSrcHasNoClass(t *testing.T) {
	for _, url := range []string{
		"https://evil.example/x.png",
		"data:image/png;base64,AAAA",
		"//evil.example/x.png",
	} {
		out := string(mustHTML(RenderDocument(context.Background(), "![x]("+url+")\n", resolveNone, nil)))
		if !strings.Contains(out, "<img") {
			t.Fatalf("%s: the <img> element itself must survive:\n%s", url, out)
		}
		if strings.Contains(out, "zoomable") {
			t.Fatalf("%s: an image with a blocked (empty) src must not be zoomable:\n%s", url, out)
		}
	}
}
