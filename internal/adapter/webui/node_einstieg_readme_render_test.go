package webui

import (
	"context"
	"html/template"
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/domain"
)

// TestEinstiegLese_ReadmeLinksLeadToRealRoutes pins the Lesespalte's two ways
// into the README: with one, "Bearbeiten" edits that document; without one,
// the single "anlegen" line opens the create editor. Neither may point at
// /nodes/{id}/readme — that address never existed and rendered the 404 page
// (Soenne, 22.08.).
func TestEinstiegLese_ReadmeLinksLeadToRealRoutes(t *testing.T) {
	t.Parallel()
	ctx := context.Background()
	n := domain.Node{ID: "eng1", Kind: domain.KindEngagement, Name: "Kunde A", Slug: "kunde-a"}

	with := NodeEinstieg{N: n, HasReadme: true, ReadmeHTML: template.HTML("<p>Hallo README</p>"),
		ReadmeHref: "/wissen/rd1/bearbeiten", ReadmePath: "kunde-a/readme", ReadmeWhen: "vor 6 Tagen"}
	out := renderToBuf(t, ctx, EinstiegLese(with))
	for _, want := range []string{`href="/wissen/rd1/bearbeiten"`, "<p>Hallo README</p>", "kunde-a/readme", "vor 6 Tagen"} {
		if !strings.Contains(out, want) {
			t.Errorf("with README: missing %q in:\n%s", want, out)
		}
	}
	if strings.Contains(out, "/nodes/eng1/readme") || strings.Contains(out, "Noch keine README") {
		t.Errorf("with README: dead link or empty line rendered:\n%s", out)
	}

	without := NodeEinstieg{N: n, DescriptionHTML: template.HTML("<p>Nur Beschreibung</p>"),
		ReadmeHref: "/wissen/neu?node=eng1&type=project&path=readme"}
	out = renderToBuf(t, ctx, EinstiegLese(without))
	for _, want := range []string{`href="/wissen/neu?node=eng1&amp;type=project&amp;path=readme"`, "<p>Nur Beschreibung</p>", "Noch keine README"} {
		if !strings.Contains(out, want) {
			t.Errorf("without README: missing %q in:\n%s", want, out)
		}
	}
	// No README: no path, no age, no end rule, no "Bearbeiten" of nothing.
	for _, bad := range []string{"/nodes/eng1/readme", "kunde-a/readme", "zuletzt", "Ende der README", ">Bearbeiten<"} {
		if strings.Contains(out, bad) {
			t.Errorf("without README: %q must not render:\n%s", bad, out)
		}
	}
}
