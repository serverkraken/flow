package webui

import (
	"context"
	"strings"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/i18n"
)

// Der Pfad entsteht aus Typ, Datum und Titel — nach der Typen-Matrix, ohne
// dass jemand ihn tippt.
func TestDerivedPath(t *testing.T) {
	now := time.Date(2026, 8, 21, 22, 0, 0, 0, time.Local)
	cases := []struct {
		typ   domain.DocumentType
		title string
		want  string
	}{
		{domain.DocProject, "Chroma-Pass für Codeblöcke", "notes/chroma-pass-fuer-codebloecke"},
		{domain.DocPlan, "Kalender Umbau", "plans/2026-08-21-kalender-umbau"},
		{domain.DocSpec, "Tourenansicht v2", "specs/2026-08-21-tourenansicht-v2-design"},
		{domain.DocMemory, "Fallen der Linie", "memories/fallen-der-linie"},
		{domain.DocDaily, "egal", "daily/2026-08-21"},
		{domain.DocFree, "", "karte"},
		{domain.DocActiveContext, "x", "active-context"},
	}
	for _, tc := range cases {
		if got := DerivedPath(tc.typ, tc.title, now); got != tc.want {
			t.Errorf("%s %q → %s, want %s", tc.typ, tc.title, got, tc.want)
		}
		if !domain.SlugOK(DerivedPath(tc.typ, tc.title, now)) {
			t.Errorf("%s: kein gültiger Slug", tc.typ)
		}
	}
}

func TestNeueKarteForm_PreselectsAndPreviews(t *testing.T) {
	ctx := i18n.WithLocale(context.Background(), i18n.DE)
	vm := NeueKarteVM{NodeID: "n1", Type: "project", Date: "2026-08-21", Types: NeueKarteTypes(ctx),
		Nodes: []EditorOption{{Value: "n1", Label: "flow"}, {Value: "n2", Label: "infra"}}}
	out := renderToBuf(t, ctx, NeueKarteForm(vm))
	for _, want := range []string{
		`action="/wissen/schnell"`, `value="n1" selected`, "vorausgewählt, weil Du hier stehst",
		`value="project" checked`, `data-pattern="plans/{date}-{slug}"`, `data-pattern="daily/{date}"`,
		"Anlegen &amp; schreiben", `data-nk-preview`, `data-nk-path`, `data-date="2026-08-21"`,
	} {
		if !strings.Contains(out, want) {
			t.Errorf("Formular ohne %q", want)
		}
	}
	if strings.Contains(out, `value="agent"`) {
		t.Errorf("der veraltete Agent-Typ gehört nicht in den Dialog")
	}
}
