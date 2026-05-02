package usecase

import (
	"errors"
	"strings"
	"time"

	"github.com/serverkraken/flow/internal/ports"
)

// LinkWriter manages per-day attachments to Kompendium notes.
type LinkWriter struct {
	Store ports.LinkStore
}

// Add attaches noteID to date. Empty IDs and IDs containing the TSV
// breakers (tab/newline/cr) are rejected. Idempotent — adding an
// existing pair is a no-op.
func (w *LinkWriter) Add(date time.Time, noteID string) error {
	noteID = strings.TrimSpace(noteID)
	if noteID == "" {
		return errors.New("note id darf nicht leer sein")
	}
	if strings.ContainsAny(noteID, "\t\n\r") {
		return errors.New("note id enthält ungültige zeichen")
	}
	return w.Store.Add(date, noteID)
}

// Remove detaches noteID from date. No-op when the pair isn't present.
func (w *LinkWriter) Remove(date time.Time, noteID string) error {
	return w.Store.Remove(date, noteID)
}
