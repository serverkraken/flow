package webui

import (
	"context"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// TestCockpitArtifacts_EmptyState verifies the gallery's empty state (Task 5
// Zustand "leer"): no own+inherited artifacts shows a quiet empty note, no
// ".gallery" grid at all, and the upload form stays visible so the owner can
// still add the first artifact.
func TestCockpitArtifacts_EmptyState(t *testing.T) {
	d := seededCockpit()
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	if !strings.Contains(out, "Noch keine Artefakte") {
		t.Fatalf("empty gallery must show the empty-state note:\n%s", out)
	}
	if strings.Contains(out, `class="gallery mt-3"`) {
		t.Fatalf("empty gallery must not render a .gallery grid:\n%s", out)
	}
	if !strings.Contains(out, `type="file"`) {
		t.Fatalf("empty gallery must still show the upload form:\n%s", out)
	}
}

// TestCockpitArtifacts_OwnCard_ImageHasThumbAndActions verifies an own
// (non-inherited) image artifact renders a thumbnail (not a chip) plus
// rename/delete/replace affordances — and that the delete affordance goes
// through a ConfirmDialog (no window.confirm popup).
func TestCockpitArtifacts_OwnCard_ImageHasThumbAndActions(t *testing.T) {
	d := seededCockpit()
	d.Artifacts = []ArtifactCardVM{
		{Slug: "logo", Name: "Logo.png", Href: "/nodes/n1/artifacts/logo?v=abc123", SizeStr: "12.0 KB", IsImage: true},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	for _, want := range []string{
		`class="artcard"`,
		`class="artcard-thumb"`,
		`src="/nodes/n1/artifacts/logo?v=abc123"`,
		"Logo.png",
		"12.0 KB",
		"Umbenennen",
		`data-dialog-open="delete-artifact-logo"`,
		`id="delete-artifact-logo"`,
		`hx-post="/nodes/n1/artifacts/logo/delete"`,
		`id="rename-artifact-logo"`,
		`hx-post="/nodes/n1/artifacts/logo/rename"`,
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

// TestCockpitArtifacts_OwnCard_NonImageHasChip verifies a non-image own
// artifact (e.g. a PDF) renders the monospace type-chip (glyph + kürzel, no
// emoji) instead of an <img> thumb.
func TestCockpitArtifacts_OwnCard_NonImageHasChip(t *testing.T) {
	d := seededCockpit()
	d.Artifacts = []ArtifactCardVM{
		{Slug: "spec", Name: "Spec.pdf", Href: "/nodes/n1/artifacts/spec", SizeStr: "2.0 KB", TypeLabel: "PDF", IsImage: false},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
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

// TestCockpitArtifacts_InheritedCard_ReadOnlyWithOrigin verifies an inherited
// card (artifact.NodeID != cockpit node) shows the origin marker and NEVER
// renders rename/delete/replace affordances — it's read-only.
func TestCockpitArtifacts_InheritedCard_ReadOnlyWithOrigin(t *testing.T) {
	d := seededCockpit()
	d.Artifacts = []ArtifactCardVM{
		{Slug: "brand", Name: "Brand.png", Href: "/nodes/root/artifacts/brand?v=ref1", SizeStr: "4.0 KB", IsImage: true, Inherited: true, FromNode: "RTL Extern"},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	if !strings.Contains(out, "geerbt von") || !strings.Contains(out, "RTL Extern") {
		t.Fatalf("inherited card must show its origin:\n%s", out)
	}
	for _, mustNotHave := range []string{
		`data-dialog-open="delete-artifact-brand"`,
		`data-dialog-open="rename-artifact-brand"`,
		"Ersetzen",
	} {
		if strings.Contains(out, mustNotHave) {
			t.Fatalf("inherited card must be read-only, found %q:\n%s", mustNotHave, out)
		}
	}
}

// TestCockpitArtifacts_MixedOwnAndInherited verifies both card kinds render
// side by side in one gallery grid (own artifacts don't hide inherited ones
// and vice versa).
func TestCockpitArtifacts_MixedOwnAndInherited(t *testing.T) {
	d := seededCockpit()
	d.Artifacts = []ArtifactCardVM{
		{Slug: "own", Name: "Own.pdf", Href: "/nodes/n1/artifacts/own", SizeStr: "1.0 KB", TypeLabel: "PDF"},
		{Slug: "inh", Name: "Inherited.pdf", Href: "/nodes/root/artifacts/inh", SizeStr: "3.0 KB", TypeLabel: "PDF", Inherited: true, FromNode: "root"},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	if !strings.Contains(out, `class="gallery mt-3"`) {
		t.Fatalf("non-empty gallery must render the .gallery grid:\n%s", out)
	}
	if !strings.Contains(out, "Own.pdf") || !strings.Contains(out, "Inherited.pdf") {
		t.Fatalf("both own and inherited cards must render:\n%s", out)
	}
	if !strings.Contains(out, `data-dialog-open="delete-artifact-own"`) {
		t.Fatalf("own card must keep its delete affordance:\n%s", out)
	}
	if strings.Contains(out, `data-dialog-open="delete-artifact-inh"`) {
		t.Fatalf("inherited card must not have a delete affordance:\n%s", out)
	}
}

// TestCockpitArtifacts_LongNameTruncates pins the "lang" state (Spec §11):
// a long display name gets the truncate/min-w-0 classes so it clips instead
// of blowing out the card width.
func TestCockpitArtifacts_LongNameTruncates(t *testing.T) {
	d := seededCockpit()
	d.Artifacts = []ArtifactCardVM{
		{Slug: "long", Name: "A-Very-Long-Descriptive-Filename-That-Should-Not-Blow-Up-The-Card-Layout.pdf", Href: "/nodes/n1/artifacts/long", SizeStr: "1.0 KB", TypeLabel: "PDF"},
	}
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	if !strings.Contains(out, `class="artcard-name truncate min-w-0"`) {
		t.Fatalf("artifact card name must carry truncate/min-w-0:\n%s", out)
	}
}

// TestCockpitArtifacts_PanelErrInline verifies an upload/rename/delete
// failure surfaces as an inline banner (role=alert) — never a popup.
func TestCockpitArtifacts_PanelErrInline(t *testing.T) {
	d := seededCockpit()
	d.PanelErr = "Datei zu groß (max. 8 MB)"
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	out := renderToBuf(t, ctx, CockpitArtifacts(d))
	if !strings.Contains(out, `role="alert"`) || !strings.Contains(out, "Datei zu groß") {
		t.Fatalf("inline error must render as a role=alert banner:\n%s", out)
	}
}

// TestBuildArtifactCards_OwnVsInherited is the VM builder's unit test: an
// artifact hanging off the cockpit's own node id is NOT inherited; one
// hanging off an ancestor id IS, with FromNode resolved from the names map.
func TestBuildArtifactCards_OwnVsInherited(t *testing.T) {
	arts := []domain.Artifact{
		{NodeID: "leaf", Slug: "own", Name: "Own.pdf", Mime: "application/pdf", SizeBytes: 100},
		{NodeID: "root", Slug: "shared", Name: "Shared.png", Mime: "image/png", SizeBytes: 200, Ref: "ref123456789"},
	}
	cards := BuildArtifactCards(arts, "leaf", map[string]string{"leaf": "Repo", "root": "RTL Extern"})
	if len(cards) != 2 {
		t.Fatalf("want 2 cards, got %d", len(cards))
	}
	own, inherited := cards[0], cards[1]
	if own.Inherited {
		t.Errorf("own artifact (NodeID=leaf) must not be marked Inherited: %+v", own)
	}
	if !inherited.Inherited || inherited.FromNode != "RTL Extern" {
		t.Errorf("ancestor artifact (NodeID=root) must be Inherited with FromNode=RTL Extern: %+v", inherited)
	}
}

// TestBuildArtifactCards_FreeArtifact is the Task 2 free-read-path test: a
// free artifact (NodeID=="") card is always Inherited (it never belongs to
// the cockpit's own node), carries the caller-supplied "Frei" origin label
// (names[""], set by the caller — Task 3), and its Href begins with
// /artefakte/ rather than /nodes/{id}/artifacts/.
func TestBuildArtifactCards_FreeArtifact(t *testing.T) {
	arts := []domain.Artifact{
		{NodeID: "", Slug: "brand", Name: "Brand.png", Mime: "image/png", SizeBytes: 300, Ref: "abcdef123456"},
	}
	cards := BuildArtifactCards(arts, "leaf", map[string]string{"": "Frei"})
	if len(cards) != 1 {
		t.Fatalf("want 1 card, got %d", len(cards))
	}
	card := cards[0]
	if !card.Inherited {
		t.Errorf("free artifact card must be Inherited: %+v", card)
	}
	if card.FromNode != "Frei" {
		t.Errorf("free artifact card FromNode = %q, want Frei: %+v", card.FromNode, card)
	}
	if !strings.HasPrefix(card.Href, "/artefakte/") {
		t.Errorf("free artifact card Href = %q, want prefix /artefakte/", card.Href)
	}
}
