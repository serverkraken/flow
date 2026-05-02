package markdown

import (
	"strings"
	"testing"

	"github.com/charmbracelet/x/ansi"
)

// TestRender_BacklinksFooter_RendersAllRefs: a non-empty slice of
// backlinks renders below the body — separator + heading + bullet
// per ref. Verifies each ref's title surfaces.
func TestRender_BacklinksFooter_RendersAllRefs(t *testing.T) {
	t.Parallel()
	refs := []BacklinkRef{
		{ID: "daily/2026-04-25", Title: "Tagesnotiz"},
		{ID: "projects/foo/_project", Title: "Foo Projekt"},
	}
	out, err := Render("body", 60, WithBacklinks(refs))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "Referenced by") {
		t.Errorf("footer heading missing:\n%s", plain)
	}
	if !strings.Contains(plain, "Tagesnotiz") || !strings.Contains(plain, "Foo Projekt") {
		t.Errorf("backlink titles missing:\n%s", plain)
	}
}

// TestRender_BacklinksFooter_EmptyOmits: an empty slice (or no
// option) skips the footer entirely — including the separator,
// which would otherwise leave a dangling rule below the body.
func TestRender_BacklinksFooter_EmptyOmits(t *testing.T) {
	t.Parallel()
	out, err := Render("body", 60, WithBacklinks(nil))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if strings.Contains(plain, "Referenced by") {
		t.Errorf("empty backlinks should not produce a footer:\n%s", plain)
	}
}

// TestRender_BacklinksFooter_FallsBackToIDWhenTitleEmpty: a backlink
// whose target has no Title (e.g. the note got deleted) renders the
// raw ID so the user knows what's pointing at the current note.
func TestRender_BacklinksFooter_FallsBackToIDWhenTitleEmpty(t *testing.T) {
	t.Parallel()
	refs := []BacklinkRef{{ID: "orphan-note", Title: ""}}
	out, err := Render("body", 60, WithBacklinks(refs))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	plain := ansi.Strip(out)
	if !strings.Contains(plain, "orphan-note") {
		t.Errorf("ID fallback missing for title-less backlink:\n%s", plain)
	}
}

// TestRender_BacklinksFooter_ResolverDecidesValidVsBroken: a backlink
// whose resolver-lookup succeeds gets ⇲ + OSC 8; a ref the resolver
// rejects gets ⌧ + dim red. Mirrors the wikilink contract so the
// reader treats both consistently.
func TestRender_BacklinksFooter_ResolverDecidesValidVsBroken(t *testing.T) {
	t.Parallel()
	refs := []BacklinkRef{
		{ID: "known", Title: "Known"},
		{ID: "missing", Title: "Missing"},
	}
	res := stubWikiResolver{known: map[string]string{"known": "kompendium://note/known"}}
	out, err := Render("body", 60, WithBacklinks(refs), WithWikilinks(res))
	if err != nil {
		t.Fatalf("Render: %v", err)
	}
	if !strings.Contains(out, "⇲ Known") {
		t.Errorf("valid backlink missing ⇲ marker:\n%s", out)
	}
	if !strings.Contains(out, "⌧ Missing") {
		t.Errorf("broken backlink missing ⌧ marker:\n%s", out)
	}
	if !strings.Contains(out, "kompendium://note/known\x07") {
		t.Errorf("valid backlink missing OSC 8 destination:\n%s", out)
	}
}
