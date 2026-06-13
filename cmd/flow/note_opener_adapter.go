package main

import (
	"context"
	"errors"

	kompdomain "github.com/serverkraken/flow/internal/kompendium/domain"
	kompusecase "github.com/serverkraken/flow/internal/kompendium/usecase"
)

// editNoteOpener adapts kompusecase.EditNote to the ports.NoteLauncher
// interface consumed by usecase.NoteOpener. It replaces the old filesystem-path
// lookup (pathOf) which always returned "" in server mode because Rooter == nil.
type editNoteOpener struct{ uc *kompusecase.EditNote }

// Open parses id into a kompdomain.ID and delegates to EditNote.Execute.
func (o *editNoteOpener) Open(id string) error {
	if id == "" {
		return errors.New("note id darf nicht leer sein")
	}
	parsed, err := kompdomain.ParseID(id)
	if err != nil {
		return err
	}
	return o.uc.Execute(context.Background(), parsed)
}
