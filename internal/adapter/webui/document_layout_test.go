package webui

import (
	"html/template"
	"os"
	"strings"
	"testing"
)

// TestDocumentFragment_LesesaalSpineProvAndRail is the Task 5 anchor test
// (plan-literal): Spine, Provenance row (actor/time/path/reading time), the
// `.read` grid, the docrail ToC, and the Anpinnen route must all be present —
// and no Kristall remnant (glass chrome, shadow, font-display utility,
// kindToneClass helper) may have survived the rewrite.
func TestDocumentFragment_LesesaalSpineProvAndRail(t *testing.T) {
	vm := DocumentVM{
		ID: "d1", Title: "Backstage ↔ GitLab: Token-Integration",
		Path: "docs/gitlab-token-integration", UpdatedByKind: "agent", UpdatedByRef: "Claude",
		ReadMinutes: 18, HTML: template.HTML("<p>x</p>"),
		Crumbs: []DocCrumb{{Label: "RTL Extern", Href: "/nodes/e1"}, {Label: "backstage", Href: "/nodes/r1"}},
	}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	for _, want := range []string{`class="spine"`, `class="prov"`, `class="provref"`, "Claude", "docs/gitlab-token-integration", "18", `class="read"`, `class="docrail"`, "data-toc-nav", "/wissen/d1/pin"} {
		if !strings.Contains(out, want) {
			t.Fatalf("doc fragment misses %q:\n%s", want, out)
		}
	}
	// Fixed after review: Delete moved off the document page entirely (to the
	// edit page, editor.templ — see TestEditorEditModeHasDeleteConfirmDialog),
	// matching the Mockup (Z.688–695: only Bearbeiten + Anpinnen). No
	// ConfirmDialog/BtnDanger lives here anymore, so the whole fragment can be
	// checked directly without any scoping.
	for _, gone := range []string{"glass", "shadow-soft", "font-display", "kindToneClass", "data-dialog-open=\"del-", "<dialog"} {
		if strings.Contains(out, gone) {
			t.Fatalf("kristall/delete remnant %q still present:\n%s", gone, out)
		}
	}
}

// TestDocumentFragmentDegradesProvRowWithoutActor covers Task 3's contract
// (domain.Document.UpdatedByKind/Ref empty for pre-provenance/legacy rows):
// the Prov row must still render time+path+reading time, just without a
// bold actor name.
func TestDocumentFragmentDegradesProvRowWithoutActor(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "T", Path: "p/x", ReadMinutes: 3, HTML: template.HTML("<p>x</p>")}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	if strings.Contains(out, `class="provref"`) {
		t.Fatalf("no actor known: provref <b> must not render:\n%s", out)
	}
	for _, want := range []string{"p/x", "3", "aktualisiert"} {
		if !strings.Contains(out, want) {
			t.Fatalf("degraded prov row misses %q:\n%s", want, out)
		}
	}
}

// TestDocumentFragmentPinButtonLabelReflectsPinned covers the Anpinnen/
// Angepinnt label toggle (Mockup Z.694) driven by DocumentVM.Pinned.
func TestDocumentFragmentPinButtonLabelReflectsPinned(t *testing.T) {
	unpinned := renderToBuf(t, testCtx(t), DocumentFragment(DocumentVM{ID: "d1", HTML: template.HTML("<p/>")}))
	if !strings.Contains(unpinned, "Anpinnen") || strings.Contains(unpinned, "Angepinnt") {
		t.Fatalf("unpinned doc must show Anpinnen, not Angepinnt:\n%s", unpinned)
	}
	pinned := renderToBuf(t, testCtx(t), DocumentFragment(DocumentVM{ID: "d1", Pinned: true, HTML: template.HTML("<p/>")}))
	if !strings.Contains(pinned, "Angepinnt") {
		t.Fatalf("pinned doc must show Angepinnt:\n%s", pinned)
	}
}

// TestDocumentFragmentReadGridHoldsProseAndDocrail replaces the Kristall-era
// TestDocumentFragmentConstrainsMarkdownColumn: the width-containment guard
// itself now lives entirely in the named `.prose`/`.read`/`.docrail` CSS
// classes (Task 1 + TestMarkdownProseCSSGuardsWideContent below), so this
// test only needs to guard the DOM shape — `.read` holds both the prose and
// the docrail, and the old glass card wrapper never comes back.
func TestDocumentFragmentReadGridHoldsProseAndDocrail(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "Wide document", HTML: template.HTML("<p>x</p>")}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	for _, want := range []string{`class="read"`, `class="prose"`, `class="docrail"`} {
		if !strings.Contains(out, want) {
			t.Fatalf("DocumentFragment missing %q in %.600s", want, out)
		}
	}
	for _, gone := range []string{"data-document-prose", "rounded-2xl glass shadow-soft"} {
		if strings.Contains(out, gone) {
			t.Fatalf("DocumentFragment must not resurrect the old Kristall card wrapper %q in %.600s", gone, out)
		}
	}
}

// TestDocumentFragmentSingleTocAfterProseInRead replaces the Kristall-era
// TestDocumentFragmentPlacesMobileTocBeforeMarkdownContent: the duplicate
// mobile/desktop ToC nav pair is retired — the Lesesaal docrail carries
// exactly one `data-toc-nav`, reflowed under the prose by CSS alone
// (web/tailwind.css `.read`/`.docrail` responsive rules, Task 1), not by a
// second DOM copy.
func TestDocumentFragmentSingleTocAfterProseInRead(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "Wide document", HTML: template.HTML("<p>x</p>")}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	if got := strings.Count(out, `data-toc-nav`); got != 1 {
		t.Fatalf("expected exactly 1 toc nav (single docrail instance, no mobile/desktop duplicate), got %d in %.800s", got, out)
	}
	for _, gone := range []string{"data-mobile-toc", "data-desktop-toc"} {
		if strings.Contains(out, gone) {
			t.Fatalf("Kristall dual mobile/desktop toc marker %q must not survive, got %.800s", gone, out)
		}
	}
	prose := strings.Index(out, `class="prose"`)
	toc := strings.Index(out, `data-toc-nav`)
	if prose < 0 || toc < 0 || prose >= toc {
		t.Fatalf("expected prose before the docrail toc; prose=%d toc=%d in %.800s", prose, toc, out)
	}
}

// TestDocumentFragmentRefsRailShowsOutgoingAndBacklinks is the Task 6 anchor
// test: an outgoing (resolved) wikilink renders as a `.krow` labelled "von
// hier", a backlink renders as a `.krow` labelled "hierher", both link to
// /wissen/<id>, and no Kristall chrome (glass/shadow-soft) survives.
func TestDocumentFragmentRefsRailShowsOutgoingAndBacklinks(t *testing.T) {
	vm := DocumentVM{
		ID: "d1", Title: "T", HTML: template.HTML("<p>x</p>"),
		Outgoing:  []RefRow{{Title: "Backstage Probleme", Href: "/wissen/d2", Dir: "document.ref.from"}},
		Backlinks: []RefRow{{Title: "Karpenter Rollout", Href: "/wissen/d3", Dir: "document.ref.to"}},
	}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	for _, want := range []string{
		`class="krow"`, `href="/wissen/d2"`, "Backstage Probleme", "von hier",
		`href="/wissen/d3"`, "Karpenter Rollout", "hierher",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("refs rail misses %q:\n%s", want, out)
		}
	}
	for _, gone := range []string{"glass", "shadow-soft"} {
		if strings.Contains(out, gone) {
			t.Fatalf("refs rail must not resurrect Kristall chrome %q:\n%s", gone, out)
		}
	}
}

// TestDocumentFragmentRefsRailEmptyShowsPlaceholder covers the empty state:
// no outgoing links and no backlinks renders the "Keine Verweise" row, not a
// bare/empty block.
func TestDocumentFragmentRefsRailEmptyShowsPlaceholder(t *testing.T) {
	vm := DocumentVM{ID: "d1", Title: "T", HTML: template.HTML("<p>x</p>")}
	out := renderToBuf(t, testCtx(t), DocumentFragment(vm))
	if !strings.Contains(out, "Keine Verweise") {
		t.Fatalf("empty refs rail must show placeholder row:\n%s", out)
	}
}

func TestMarkdownProseCSSGuardsWideContent(t *testing.T) {
	css, err := os.ReadFile("../../../web/tailwind.css")
	if err != nil {
		t.Fatal(err)
	}
	src := string(css)
	for _, want := range []string{
		"overflow-wrap: anywhere;",
		".prose a { @apply text-accent underline underline-offset-2 break-words; }",
		".prose code {",
		"word-break: break-word;",
		".prose pre {",
		"max-width: 100%;",
		"overflow-x: auto;",
		".prose table {",
		"display: block;",
	} {
		if !strings.Contains(src, want) {
			t.Fatalf("web/tailwind.css missing markdown overflow guard %q", want)
		}
	}
}
