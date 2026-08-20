package webui

// Sortieren nach Katalog 3.10. Angeboten wird NUR, was das Datenmodell
// hergibt: geändert und angelegt sind echte Spalten, "zuletzt gelesen" und
// "zuletzt gezogen" aus dem Konzept gibt es hier nicht — dafür UI zu bauen
// hieße, dem Leser eine Wahl vorzugaukeln, die nichts sortiert.

import (
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

func sortFixture() []domain.Document {
	t0 := time.Date(2026, 8, 1, 12, 0, 0, 0, time.UTC)
	return []domain.Document{
		{ID: "b", Title: "Beta", Type: domain.DocSpec, CreatedAt: t0.AddDate(0, 0, -10), UpdatedAt: t0.AddDate(0, 0, -1)},
		{ID: "a", Title: "Alpha", Type: domain.DocPlan, CreatedAt: t0.AddDate(0, 0, -2), UpdatedAt: t0.AddDate(0, 0, -3)},
		{ID: "c", Title: "Gamma", Type: domain.DocPlan, CreatedAt: t0.AddDate(0, 0, -5), UpdatedAt: t0},
	}
}

func ids(docs []domain.Document) []string {
	out := make([]string, 0, len(docs))
	for _, d := range docs {
		out = append(out, d.ID)
	}
	return out
}

func eq(a, b []string) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

func TestSortDocuments(t *testing.T) {
	cases := []struct {
		mode DocSort
		want []string
	}{
		{SortChanged, []string{"c", "b", "a"}}, // zuletzt geändert, neuste zuerst
		{SortCreated, []string{"a", "c", "b"}}, // angelegt, neuste zuerst
		{SortTitle, []string{"a", "b", "c"}},   // Titel A→Z
	}
	for _, c := range cases {
		got := ids(SortDocuments(sortFixture(), c.mode))
		if !eq(got, c.want) {
			t.Errorf("SortDocuments(%q) = %v, erwartet %v", c.mode, got, c.want)
		}
	}
}

// "Typ · dann Datum": innerhalb eines Typs gilt wieder das Änderungsdatum.
func TestSortDocuments_ByTypeThenDate(t *testing.T) {
	got := ids(SortDocuments(sortFixture(), SortType))
	if got[len(got)-1] != "b" {
		t.Errorf("Spec-Karte muss hinter den Plänen stehen: %v", got)
	}
	if !eq(got[:2], []string{"c", "a"}) {
		t.Errorf("innerhalb des Typs gilt das Datum: %v", got)
	}
}

// Die Eingabe darf nicht verändert werden — die Liste wird an mehreren
// Stellen weiterverwendet.
func TestSortDocuments_DoesNotMutateInput(t *testing.T) {
	in := sortFixture()
	before := ids(in)
	SortDocuments(in, SortTitle)
	if !eq(ids(in), before) {
		t.Errorf("Eingabe wurde umsortiert: %v → %v", before, ids(in))
	}
}

// Unbekannte oder fehlende Werte fallen auf den Standard zurück, nie auf 404.
func TestNormalizeDocSort(t *testing.T) {
	for raw, want := range map[string]DocSort{
		"":         SortChanged,
		"angelegt": SortCreated,
		"titel":    SortTitle,
		"typ":      SortType,
		"quatsch":  SortChanged,
	} {
		if got := NormalizeDocSort(raw); got != want {
			t.Errorf("NormalizeDocSort(%q) = %q, erwartet %q", raw, got, want)
		}
	}
}
