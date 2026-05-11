package main

import (
	"context"
	"os"

	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompendiumcli "github.com/serverkraken/flow/internal/kompendium/frontend/cli"
	kompports "github.com/serverkraken/flow/internal/kompendium/ports"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
)

// kompendiumNoteLister adaptiert die kompendium-Subtree-ListNotes-
// Usecase auf das schmale worktime.NoteLister-Interface, das der
// Heute-Tab für seinen Note-Attach-Picker konsumiert. Der Worktime-
// Screen bleibt damit unabhängig von der Kompendium-Typenkette
// (NoteEntry, Frontmatter, …); die Konvertierung passiert hier am
// Composition-Root.
type kompendiumNoteLister struct {
	listNotes   *kompusecase.ListNotes
	currentRepo kompdomain.CanonicalURL
}

// newKompendiumNoteLister baut den Adapter mit einem vom Composition
// Root vorabgelösten currentRepo (siehe detectCurrentRepo in main.go).
// Aus dem Repo leitet ListNotes die Tier-Promotion (project notes des
// current repos zuerst) ab — das macht die Picker-Liste „relevant
// zuerst". Wenn der Adapter ohne laufenden Kompendium-Notebook gebaut
// wird (kein Index, kein Store) oder kein Repo erkannt wird, gibt
// Recent ein nil-Slice zurück; der Worktime-Picker degradiert dann
// sauber zur Reine-ID-Eingabe.
func newKompendiumNoteLister(kompDeps kompendiumcli.Deps, currentRepo kompdomain.CanonicalURL) *kompendiumNoteLister {
	return &kompendiumNoteLister{listNotes: kompDeps.ListNotes, currentRepo: currentRepo}
}

// detectCurrentRepo resolves the current working directory to its
// canonical repo URL via kompDeps.Repo.Detect. Centralised so the note
// lister adapter and the notes screen factory share one snapshot
// instead of each doing their own os.Getwd + Detect round-trip.
//
// Returns the zero CanonicalURL on any error (no repo, missing CWD,
// detector unavailable). Downstream callers treat that as "no repo
// context" and skip tier-promotion.
func detectCurrentRepo(kompDeps kompendiumcli.Deps) kompdomain.CanonicalURL {
	if kompDeps.Repo == nil {
		return ""
	}
	cwd, err := os.Getwd()
	if err != nil {
		return ""
	}
	info, derr := kompDeps.Repo.Detect(context.Background(), cwd)
	if derr != nil {
		return ""
	}
	return info.URL
}

// kompendiumNoteReader adaptiert kompendium ports.NoteStore.Get auf
// das schmale worktime.NoteReader-Interface, das der integrierte
// Note-Viewer in Heute (`o`-Key) konsumiert. Liefert den Markdown-Body
// einer Note als string; Composition-Root-Wiring isoliert den Worktime-
// Screen vom kompendium domain.Note Typ.
type kompendiumNoteReader struct {
	store kompports.NoteStore
}

// newKompendiumNoteReader baut den Adapter über kompDeps.Store. Bei
// nil-Store (kompendium nicht initialisiert) liefert Read einen Fehler.
func newKompendiumNoteReader(kompDeps kompendiumcli.Deps) *kompendiumNoteReader {
	return &kompendiumNoteReader{store: kompDeps.Store}
}

// Read implementiert worktime.NoteReader. Parst die ID, lädt die Note
// über den Store und gibt den Body als string zurück. Parse-Fehler und
// Store-Fehler propagieren nach oben — der Heute-Note-Viewer rendert
// sie inline.
func (a *kompendiumNoteReader) Read(id string) (string, error) {
	parsed, err := kompdomain.ParseID(id)
	if err != nil {
		return "", err
	}
	note, err := a.store.Get(context.Background(), parsed)
	if err != nil {
		return "", err
	}
	return string(note.Body), nil
}

// Recent implementiert worktime.NoteLister. Liefert bis zu `limit`
// jüngste Notes — Tier-Reihenfolge: project notes des aktuellen Repos
// zuerst, dann daily, dann der Rest. Innerhalb eines Tiers nach mtime
// DESC. Fehler werden geschluckt: ein kaputter Adapter darf den
// Heute-Tab nicht blanken.
func (a *kompendiumNoteLister) Recent(limit int) []worktime.NoteSuggestion {
	if a == nil || a.listNotes == nil {
		return nil
	}
	if limit <= 0 {
		return nil
	}
	entries, err := a.listNotes.Execute(context.Background(), kompusecase.ListNotesInput{
		CurrentRepo: a.currentRepo,
		Limit:       limit,
	})
	if err != nil {
		return nil
	}
	out := make([]worktime.NoteSuggestion, 0, len(entries))
	for _, e := range entries {
		title := e.Meta.Title
		if title == "" {
			title = string(e.ID)
		}
		out = append(out, worktime.NoteSuggestion{
			ID:    string(e.ID),
			Title: title,
		})
	}
	return out
}
