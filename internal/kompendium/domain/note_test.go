package domain_test

import (
	"errors"
	"reflect"
	"testing"

	"github.com/serverkraken/flow/internal/kompendium/domain"
)

func TestNewNote_Valid(t *testing.T) {
	t.Parallel()

	id := domain.ID("daily/2026-04-25")
	meta := domain.Frontmatter{ID: "daily/2026-04-25", Type: domain.TypeDaily}
	body := []byte("# heading\n")

	note, err := domain.NewNote(id, meta, body)
	if err != nil {
		t.Fatalf("NewNote: %v", err)
	}
	if note.ID != id {
		t.Errorf("ID got %q, want %q", note.ID, id)
	}
	if !reflect.DeepEqual(note.Meta, meta) {
		t.Errorf("Meta got %+v, want %+v", note.Meta, meta)
	}
	if !reflect.DeepEqual(note.Body, body) {
		t.Errorf("Body got %q, want %q", note.Body, body)
	}
}

func TestNewNote_InvalidMeta(t *testing.T) {
	t.Parallel()

	_, err := domain.NewNote(domain.ID("x"), domain.Frontmatter{Type: domain.TypeDaily}, nil)
	if !errors.Is(err, domain.ErrInvalidFrontmatter) {
		t.Errorf("expected ErrInvalidFrontmatter, got %v", err)
	}
}

func TestNote_Links(t *testing.T) {
	t.Parallel()

	note := domain.Note{Body: []byte("see [[a]] and [[b|display]]")}
	got := note.Links()
	want := []domain.Link{{Target: "a"}, {Target: "b", Display: "display"}}
	if !reflect.DeepEqual(got, want) {
		t.Errorf("got %#v, want %#v", got, want)
	}
}
