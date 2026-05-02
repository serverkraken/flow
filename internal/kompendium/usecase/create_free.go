package usecase

import (
	"context"
	"errors"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// ErrSlugRequired signals that CreateFree was called without a slug.
var ErrSlugRequired = errors.New("slug is required")

// CreateFree creates a free-form note at "notes/<slug>" and opens it.
//
// Index is optional — when set, the note is upserted into the FTS5 index
// after the editor closes so search hits the new content immediately.
type CreateFree struct {
	Store  ports.NoteStore
	Editor ports.Editor
	Index  ports.Indexer
}

// NewCreateFree wires the use case with its required ports.
func NewCreateFree(store ports.NoteStore, editor ports.Editor) *CreateFree {
	return &CreateFree{Store: store, Editor: editor}
}

// CreateFreeInput carries the slug supplied by the user.
type CreateFreeInput struct {
	Slug  string
	Title string
}

// CreateFreeOutput mirrors the other Create* outputs.
type CreateFreeOutput struct {
	ID      domain.ID
	Created bool
	Path    string
}

// Execute parses the slug into an ID under "notes/", creates the note if
// missing, and opens it.
func (u *CreateFree) Execute(ctx context.Context, in CreateFreeInput) (CreateFreeOutput, error) {
	if in.Slug == "" {
		return CreateFreeOutput{}, ErrSlugRequired
	}
	id, err := domain.ParseID("notes/" + in.Slug)
	if err != nil {
		return CreateFreeOutput{}, fmt.Errorf("invalid slug: %w", err)
	}

	exists, err := u.Store.Exists(ctx, id)
	if err != nil {
		return CreateFreeOutput{}, fmt.Errorf("exists: %w", err)
	}

	if !exists {
		note, err := buildFreeTemplate(id, in.Title)
		if err != nil {
			return CreateFreeOutput{}, err
		}
		if err := u.Store.Put(ctx, note); err != nil {
			return CreateFreeOutput{}, fmt.Errorf("put: %w", err)
		}
	}

	path := u.Store.Path(id)
	if err := u.Editor.Edit(ctx, path); err != nil {
		return CreateFreeOutput{}, fmt.Errorf("edit: %w", err)
	}
	reindex(ctx, u.Store, u.Index, id)
	return CreateFreeOutput{ID: id, Created: !exists, Path: path}, nil
}

func buildFreeTemplate(id domain.ID, title string) (domain.Note, error) {
	return domain.NewNote(id, domain.Frontmatter{
		ID:    id.String(),
		Type:  domain.TypeFree,
		Title: title,
	}, []byte{})
}
