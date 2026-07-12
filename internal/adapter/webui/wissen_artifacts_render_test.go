package webui

// Free-artifact gallery render tests (Task 4, /wissen/artefakte) — in package
// webui so they can reach the unexported freeArtifactCard/freeArtifactUploadForm
// component functions. Mirrors cockpit_artifacts_render_test.go's structure,
// but every card here is "own" (a free artifact's NodeID=="" always equals the
// nodeID="" passed to BuildArtifactCards, so Inherited is always false) —
// there is no inherited/read-only state on this page, unlike the cockpit
// gallery.

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// TestWissenArtifactsFragment_EmptyState verifies the "leer" Zustand: no free
// artifacts shows a quiet empty note (distinct wording from the cockpit
// gallery's "Noch keine Artefakte."), no .gallery grid, and the upload form
// stays visible.
func TestWissenArtifactsFragment_EmptyState(t *testing.T) {
	vm := WissenArtifactsVM{}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	if !strings.Contains(out, "Noch keine freien Artefakte") {
		t.Fatalf("empty gallery must show the free-specific empty-state note:\n%s", out)
	}
	if strings.Contains(out, `class="gallery mt-3"`) {
		t.Fatalf("empty gallery must not render a .gallery grid:\n%s", out)
	}
	if !strings.Contains(out, `type="file"`) {
		t.Fatalf("empty gallery must still show the upload form:\n%s", out)
	}
}

// TestWissenArtifactsFragment_OwnCard_ImageHasThumbAndActions verifies a free
// image artifact renders a thumbnail (not a chip) plus full rename/delete/
// replace affordances — via ConfirmDialog, never window.confirm().
func TestWissenArtifactsFragment_OwnCard_ImageHasThumbAndActions(t *testing.T) {
	vm := WissenArtifactsVM{Cards: []ArtifactCardVM{
		{Slug: "logo", Name: "Logo.png", Href: "/artefakte/logo?v=abc123", SizeStr: "12.0 KB", IsImage: true},
	}}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	for _, want := range []string{
		`class="artcard"`,
		`class="artcard-thumb"`,
		`src="/artefakte/logo?v=abc123"`,
		"Logo.png",
		"12.0 KB",
		"Umbenennen",
		`data-dialog-open="delete-free-artifact-logo"`,
		`id="delete-free-artifact-logo"`,
		`hx-post="/wissen/artefakte/logo/delete"`,
		`id="rename-free-artifact-logo"`,
		`hx-post="/wissen/artefakte/logo/rename"`,
		"Ersetzen",
	} {
		if !strings.Contains(out, want) {
			t.Fatalf("own image card misses %q:\n%s", want, out)
		}
	}
	if strings.Contains(out, "artcard-chip") {
		t.Fatalf("an image card must not render the file-chip variant:\n%s", out)
	}
}

// TestWissenArtifactsFragment_OwnCard_NonImageHasChip verifies a non-image
// artifact (e.g. a PDF) renders the monospace type-chip (glyph + kürzel, no
// emoji) instead of an <img> thumb, and reuses artifactTypeLabel's chip glyph.
func TestWissenArtifactsFragment_OwnCard_NonImageHasChip(t *testing.T) {
	vm := WissenArtifactsVM{Cards: []ArtifactCardVM{
		{Slug: "spec", Name: "Spec.pdf", Href: "/artefakte/spec", SizeStr: "2.0 KB", TypeLabel: "PDF", IsImage: false},
	}}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	if !strings.Contains(out, `class="artcard-thumb artcard-chip"`) {
		t.Fatalf("non-image card must render the chip variant:\n%s", out)
	}
	if !strings.Contains(out, "▤ PDF") {
		t.Fatalf("chip must show the monospace glyph + type kürzel:\n%s", out)
	}
	if strings.Contains(out, "<img") {
		t.Fatalf("non-image card must not render an <img> thumb:\n%s", out)
	}
}

// TestWissenArtifactsFragment_MultipleCards verifies several free cards
// render side by side in one gallery grid — this page has no
// own-vs-inherited split, every card carries full actions.
func TestWissenArtifactsFragment_MultipleCards(t *testing.T) {
	vm := WissenArtifactsVM{Cards: []ArtifactCardVM{
		{Slug: "one", Name: "One.pdf", Href: "/artefakte/one", SizeStr: "1.0 KB", TypeLabel: "PDF"},
		{Slug: "two", Name: "Two.pdf", Href: "/artefakte/two", SizeStr: "3.0 KB", TypeLabel: "PDF"},
	}}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	if !strings.Contains(out, `class="gallery mt-3"`) {
		t.Fatalf("non-empty gallery must render the .gallery grid:\n%s", out)
	}
	if !strings.Contains(out, "One.pdf") || !strings.Contains(out, "Two.pdf") {
		t.Fatalf("both cards must render:\n%s", out)
	}
	if !strings.Contains(out, `data-dialog-open="delete-free-artifact-one"`) ||
		!strings.Contains(out, `data-dialog-open="delete-free-artifact-two"`) {
		t.Fatalf("both cards must keep their delete affordance (no inherited/read-only state here):\n%s", out)
	}
}

// TestWissenArtifactsFragment_LongNameTruncates pins the "lang" state: a long
// display name gets the truncate/min-w-0 classes so it clips instead of
// blowing out the card layout.
func TestWissenArtifactsFragment_LongNameTruncates(t *testing.T) {
	vm := WissenArtifactsVM{Cards: []ArtifactCardVM{
		{Slug: "long", Name: "A-Very-Long-Descriptive-Filename-That-Should-Not-Blow-Up-The-Card-Layout.pdf", Href: "/artefakte/long", SizeStr: "1.0 KB", TypeLabel: "PDF"},
	}}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	if !strings.Contains(out, `class="artcard-name truncate min-w-0"`) {
		t.Fatalf("artifact card name must carry truncate/min-w-0:\n%s", out)
	}
}

// TestWissenArtifactsFragment_PanelErrInline verifies an upload/rename/delete
// failure surfaces as an inline role=alert banner — never a popup.
func TestWissenArtifactsFragment_PanelErrInline(t *testing.T) {
	vm := WissenArtifactsVM{PanelErr: "Datei zu groß (max. 8 MB)"}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, WissenArtifactsFragment(vm))
	if !strings.Contains(out, `role="alert"`) || !strings.Contains(out, "Datei zu groß") {
		t.Fatalf("inline error must render as a role=alert banner:\n%s", out)
	}
}

// TestWissenArtifactsOuter_SSEContainerTargetsFragmentRoute is the CRITICAL
// regression guard for gemini-Fund #2: the #wissen-artefakte SSE container's
// hx-get MUST point at the fragment-only route /ui/wissen/artefakte, NEVER at
// the full page route /wissen/artefakte — otherwise an SSE trigger would swap
// a whole AppShell page into the container div (nested page).
func TestWissenArtifactsOuter_SSEContainerTargetsFragmentRoute(t *testing.T) {
	vm := WissenArtifactsVM{}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, wissenArtifactsOuter(vm))
	if !strings.Contains(out, `id="wissen-artefakte"`) {
		t.Fatalf("outer must mount the #wissen-artefakte SSE container:\n%s", out)
	}
	if !strings.Contains(out, `hx-get="/ui/wissen/artefakte"`) {
		t.Fatalf("SSE container must hx-get the FRAGMENT route /ui/wissen/artefakte:\n%s", out)
	}
	if strings.Contains(out, `hx-get="/wissen/artefakte"`) {
		t.Fatalf("SSE container must NOT hx-get the full PAGE route /wissen/artefakte:\n%s", out)
	}
	for _, trig := range []string{"sse:artifact.created", "sse:artifact.updated", "sse:artifact.deleted"} {
		if !strings.Contains(out, trig) {
			t.Fatalf("SSE container missing trigger %q:\n%s", trig, out)
		}
	}
}

// TestBuildWissenArtifactsVM_FreeCardsAreOwnNotInherited is the VM builder's
// unit test: BuildArtifactCards called with nodeID=="" marks every free card
// (NodeID=="") as NOT inherited — this is the owner's own library, so
// rename/delete/replace affordances must always render.
func TestBuildWissenArtifactsVM_FreeCardsAreOwnNotInherited(t *testing.T) {
	arts := []domain.Artifact{
		{NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png", SizeBytes: 300, Ref: "abcdef123456"},
	}
	vm := BuildWissenArtifactsVM(arts, "")
	if len(vm.Cards) != 1 {
		t.Fatalf("want 1 card, got %d", len(vm.Cards))
	}
	card := vm.Cards[0]
	if card.Inherited {
		t.Errorf("a free artifact in its OWN gallery must not be marked Inherited: %+v", card)
	}
	if !strings.HasPrefix(card.Href, "/artefakte/") {
		t.Errorf("free artifact card Href = %q, want prefix /artefakte/", card.Href)
	}
}
