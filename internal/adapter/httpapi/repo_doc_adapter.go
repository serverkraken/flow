package httpapi

// RepoDocAdapter bridges usecase.RepoNotes (which expects ports.RepoStore +
// ports.RepoNoteStore) onto the Documents bearer API in server-only mode.
//
// Design: in the new architecture repos have no separate server-side table
// exposed to the client — the canonical key IS the address. The Documents
// adapter speaks to /api/v1/repos/<key>/note directly. Two separate types
// implement the two ports:
//
//	RepoStoreAdapter     → ports.RepoStore
//	RepoNoteStoreAdapter → ports.RepoNoteStore
//
// NewRepoDocAdapter returns a single struct that satisfies both ports by
// embedding them, and whose pointer satisfies both interfaces so it can
// be passed to usecase.NewRepoNotes for both args.

import (
	"errors"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
)

// RepoDocAdapter wraps *Documents and satisfies both ports.RepoStore and
// ports.RepoNoteStore for use in usecase.NewRepoNotes. Only the methods
// called by usecase.RepoNotes.GetForPwd and .Save are implemented;
// the sync-oriented PullSince methods return empty results.
//
// Because both ports define Upsert and PullSince with different signatures,
// we use two separate concrete types that share the same Documents backend.
type RepoDocAdapter struct {
	docs *Documents
}

// NewRepoDocAdapter constructs a RepoDocAdapter backed by d.
func NewRepoDocAdapter(d *Documents) *RepoDocAdapter {
	return &RepoDocAdapter{docs: d}
}

// RepoStore returns a ports.RepoStore view of this adapter.
func (a *RepoDocAdapter) RepoStore() ports.RepoStore { return (*repoStoreShim)(a) }

// RepoNoteStore returns a ports.RepoNoteStore view of this adapter.
func (a *RepoDocAdapter) RepoNoteStore() ports.RepoNoteStore { return (*repoNoteStoreShim)(a) }

// — ports.RepoStore shim ------------------------------------------------------

type repoStoreShim RepoDocAdapter

// EnsureByCanonicalKey returns a synthetic Repo whose ID equals the
// canonical key. No server round-trip is needed — the Documents API
// addresses notes by repoKey directly.
func (s *repoStoreShim) EnsureByCanonicalKey(_, key, displayName string) (domain.Repo, error) {
	return domain.Repo{
		ID:           key,
		CanonicalKey: key,
		DisplayName:  displayName,
		CreatedAt:    time.Time{},
	}, nil
}

// GetByID is not called by usecase.RepoNotes but required by ports.RepoStore.
func (s *repoStoreShim) GetByID(_, id string) (domain.Repo, error) {
	return domain.Repo{ID: id, CanonicalKey: id}, nil
}

// Upsert is not called by usecase.RepoNotes in server mode (no sync queue).
func (s *repoStoreShim) Upsert(_ domain.Repo) error { return nil }

// PullSince is the sync-worker pagination method. Returns empty in R2a.
func (s *repoStoreShim) PullSince(_ string, _ int64, _ int) ([]domain.Repo, int64, bool, error) {
	return nil, 0, false, nil
}

// — ports.RepoNoteStore shim --------------------------------------------------

type repoNoteStoreShim RepoDocAdapter

// GetByRepo fetches the note for repoID (== canonical key) via the Documents
// bearer API. Returns ports.ErrRepoNoteNotFound when no note exists yet.
func (s *repoNoteStoreShim) GetByRepo(_, repoID string) (domain.RepoNote, error) {
	doc, err := s.docs.GetByRepoKey("", repoID)
	if err != nil {
		if errors.Is(err, ports.ErrDocumentNotFound) {
			return domain.RepoNote{}, ports.ErrRepoNoteNotFound
		}
		return domain.RepoNote{}, err
	}
	return domain.RepoNote{
		ID:        doc.Path, // path serves as stable ID from the server
		RepoID:    repoID,
		Content:   doc.Body,
		Version:   doc.Version,
		UpdatedAt: doc.UpdatedAt,
	}, nil
}

// Upsert saves the note content for n.RepoID (== canonical key) via
// Documents.Put with If-Match semantics (n.Version).
func (s *repoNoteStoreShim) Upsert(n domain.RepoNote) error {
	_, err := s.docs.Put("", "", n.Content, n.RepoID, n.Version)
	if err != nil {
		if errors.Is(err, ports.ErrDocumentVersionConflict) {
			return ports.ErrRepoNoteVersionConflict
		}
		return err
	}
	return nil
}

// Delete removes the note for the given ID. Idempotent: missing note is not an error.
func (s *repoNoteStoreShim) Delete(_, id string) error {
	return s.docs.Delete("", id)
}

// PullSince is the sync-worker pagination method. Returns empty in R2a.
func (s *repoNoteStoreShim) PullSince(_ string, _ int64, _ int) ([]domain.RepoNote, int64, bool, error) {
	return nil, 0, false, nil
}
