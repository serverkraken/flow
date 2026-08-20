package pgstore

// Die Sortierwahl kommt aus der URL. Sie darf niemals als Text ins SQL
// wandern — deshalb eine reine Whitelist, und deshalb dieser Test.

import (
	"strings"
	"testing"

	"github.com/serverkraken/flow/internal/ports"
)

func TestLibraryOrderBy_Whitelist(t *testing.T) {
	cases := map[ports.DocumentLibrarySort]string{
		ports.DocumentLibrarySortChanged: "d.updated_at DESC",
		ports.DocumentLibrarySortCreated: "d.created_at DESC",
		ports.DocumentLibrarySortTitle:   "lower(d.title) ASC",
		ports.DocumentLibrarySortType:    "d.type ASC",
	}
	for sort, want := range cases {
		got := libraryOrderBy(sort, ports.DocumentLibraryActive)
		if !strings.HasPrefix(got, want) {
			t.Errorf("libraryOrderBy(%q) = %q, erwartet Beginn %q", sort, got, want)
		}
		if !strings.HasSuffix(got, "d.id ASC") {
			t.Errorf("libraryOrderBy(%q) = %q — d.id ASC fehlt, Seitenkanten wackeln", sort, got)
		}
	}
}

// Ein erfundener Wert darf nicht durchschlagen.
func TestLibraryOrderBy_UnknownFallsBackToDefault(t *testing.T) {
	got := libraryOrderBy(ports.DocumentLibrarySort("d.title; DROP TABLE documents"), ports.DocumentLibraryActive)
	if got != "d.updated_at DESC, d.id ASC" {
		t.Errorf("unbekannte Wahl muss auf den Standard fallen, got %q", got)
	}
}

// Das Archiv behält seine eigene Standardordnung.
func TestLibraryOrderBy_ArchivedKeepsItsDefault(t *testing.T) {
	got := libraryOrderBy(ports.DocumentLibrarySortChanged, ports.DocumentLibraryArchived)
	if !strings.HasPrefix(got, "d.archived_at DESC") {
		t.Errorf("Archiv sortiert nach Archivdatum, got %q", got)
	}
}
