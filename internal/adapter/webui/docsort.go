package webui

import (
	"sort"
	"strings"

	"github.com/serverkraken/flow/internal/adapter/webui/components"
	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// DocSort ist die bewusste Sortierwahl einer Liste (Katalog 3.10). Der
// Standard ist "zuletzt geändert" und trägt den leeren Wert, damit die
// unsortierte URL die Standard-URL ist — keine ?sort=geaendert-Rauschzeile in
// jedem Link.
//
// Das Konzept nennt vier Zeitpunkte; angeboten werden hier nur die beiden,
// die das Datenmodell führt. "zuletzt gelesen" und "zuletzt gezogen"
// existieren auf dieser Basis nicht — eine Wahl anzubieten, die nichts
// sortiert, wäre schlimmer als sie wegzulassen.
type DocSort string

const (
	SortChanged DocSort = ""         // zuletzt geändert — Standard in jeder Liste
	SortCreated DocSort = "angelegt" // angelegt, ändert sich nie
	SortTitle   DocSort = "titel"    // Titel · A→Z
	SortType    DocSort = "typ"      // Typ · dann Datum
)

// DocSortOptions ist die Reihenfolge, in der die Wahl im Menü steht.
var DocSortOptions = []struct {
	Mode     DocSort
	LabelKey string // Wort im Menü
	HeadKey  string // Wort in der Spaltenüberschrift, wenn aktiv
}{
	{SortChanged, "sort.changed", "sort.head.changed"},
	{SortCreated, "sort.created", "sort.head.created"},
	{SortTitle, "sort.title", "sort.head.title"},
	{SortType, "sort.type", "sort.head.type"},
}

// NormalizeDocSort bildet einen rohen Query-Wert auf eine Wahl ab. Unbekanntes
// fällt auf den Standard — eine kaputte URL soll die Liste nicht zerreißen.
func NormalizeDocSort(raw string) DocSort {
	for _, o := range DocSortOptions {
		if o.Mode != SortChanged && string(o.Mode) == raw {
			return o.Mode
		}
	}
	return SortChanged
}

// SortHeadKey ist das Wort, das die Spaltenüberschrift trägt. Ist eine andere
// Wahl aktiv, sagt die Überschrift das an — sie heißt dann "angelegt", nicht
// weiter "geändert" (Katalog 3.10).
func SortHeadKey(mode DocSort) string {
	for _, o := range DocSortOptions {
		if o.Mode == mode {
			return o.HeadKey
		}
	}
	return "sort.head.changed"
}

// SortDocuments sortiert eine Kopie — die Eingabeliste wird an mehreren
// Stellen weiterverwendet und darf sich nicht unter der Hand umordnen.
func SortDocuments(docs []domain.Document, mode DocSort) []domain.Document {
	out := append([]domain.Document(nil), docs...)
	switch mode {
	case SortCreated:
		sort.SliceStable(out, func(i, j int) bool { return out[j].CreatedAt.Before(out[i].CreatedAt) })
	case SortTitle:
		sort.SliceStable(out, func(i, j int) bool { return out[i].Title < out[j].Title })
	case SortType:
		sort.SliceStable(out, func(i, j int) bool {
			if out[i].Type != out[j].Type {
				return out[i].Type < out[j].Type
			}
			return out[j].UpdatedAt.Before(out[i].UpdatedAt)
		})
	default:
		sort.SliceStable(out, func(i, j int) bool { return out[j].UpdatedAt.Before(out[i].UpdatedAt) })
	}
	return out
}

// BuildSortHead baut den sichtbaren Schalter zur aktuellen Wahl. baseHref ist
// die URL der Liste ohne sort-Parameter; der Standard bekommt bewusst KEINEN
// Parameter, damit die unsortierte URL die Standard-URL bleibt.
func BuildSortHead(mode DocSort, baseHref string) components.SortHeadVM {
	sep := "?"
	if strings.Contains(baseHref, "?") {
		sep = "&"
	}
	opts := make([]components.SortHeadOption, 0, len(DocSortOptions))
	for _, o := range DocSortOptions {
		href := baseHref
		if o.Mode != SortChanged {
			href = baseHref + sep + "sort=" + string(o.Mode)
		}
		opts = append(opts, components.SortHeadOption{
			LabelKey: o.LabelKey, Href: href, Active: o.Mode == mode,
		})
	}
	return components.SortHeadVM{HeadKey: SortHeadKey(mode), Options: opts}
}

// LibrarySort übersetzt die UI-Wahl in die Store-Wahl. Zwei Aufzählungen,
// weil die eine in URLs steht und die andere in einer Abfrage — sie sollen
// sich unabhängig ändern dürfen.
func LibrarySort(mode DocSort) ports.DocumentLibrarySort {
	switch mode {
	case SortCreated:
		return ports.DocumentLibrarySortCreated
	case SortTitle:
		return ports.DocumentLibrarySortTitle
	case SortType:
		return ports.DocumentLibrarySortType
	}
	return ports.DocumentLibrarySortChanged
}
