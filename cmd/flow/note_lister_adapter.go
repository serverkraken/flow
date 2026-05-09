package main

import (
	"context"
	"os"

	"github.com/serverkraken/flow/internal/frontend/tui/screen/worktime"
	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompendiumcli "github.com/serverkraken/flow/internal/kompendium/frontend/cli"
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

// newKompendiumNoteLister baut den Adapter mit einem Snapshot des
// aktuellen Repos zur Boot-Zeit. Aus dem Repo leitet ListNotes die
// Tier-Promotion (project notes des current repos zuerst) ab — das
// macht die Picker-Liste „relevant zuerst". Wenn der Adapter ohne
// laufenden Kompendium-Notebook gebaut wird (kein Index, kein Store),
// gibt Recent ein nil-Slice zurück; der Worktime-Picker degradiert
// dann sauber zur Reine-ID-Eingabe.
func newKompendiumNoteLister(kompDeps kompendiumcli.Deps) *kompendiumNoteLister {
	a := &kompendiumNoteLister{listNotes: kompDeps.ListNotes}
	if kompDeps.Repo != nil {
		cwd, err := os.Getwd()
		if err == nil {
			if info, derr := kompDeps.Repo.Detect(context.Background(), cwd); derr == nil {
				a.currentRepo = info.URL
			}
		}
	}
	return a
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
