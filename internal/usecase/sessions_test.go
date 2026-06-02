package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeSessionsProjectStore is a minimal ports.ProjectStore for Sessions tests.
// It is separate from fakeProjectStore to avoid coupling across test files.
type fakeSessionsProjectStore struct {
	projects   []domain.Project
	getByIDErr error
	ensureErr  error
	upserted   []domain.Project
	touched    []string
	archived   []string
}

func (f *fakeSessionsProjectStore) ListActive(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeSessionsProjectStore) ListAll(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeSessionsProjectStore) GetByID(_, id string) (domain.Project, error) {
	if f.getByIDErr != nil {
		return domain.Project{}, f.getByIDErr
	}
	for _, p := range f.projects {
		if p.ID == id {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeSessionsProjectStore) GetBySlug(_, slug string) (domain.Project, error) {
	for _, p := range f.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeSessionsProjectStore) EnsureBySlug(_, name, slug string) (domain.Project, error) {
	if f.ensureErr != nil {
		return domain.Project{}, f.ensureErr
	}
	p := domain.Project{ID: "auto-id", Name: name, Slug: slug, CreatedAt: time.Now()}
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeSessionsProjectStore) Upsert(p domain.Project) error {
	f.upserted = append(f.upserted, p)
	return nil
}

func (f *fakeSessionsProjectStore) TouchLastUsed(_, id string) error {
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeSessionsProjectStore) Archive(_, id string) error {
	f.archived = append(f.archived, id)
	return nil
}

// fakeSessionsStore is a minimal ports.SessionStore for Sessions tests.
// It is separate from fakeSessionStore/flakySessionStore to isolate concerns.
type fakeSessionsStore struct {
	sessions  []domain.Session
	upserted  []domain.Session
	deleted   []string
	upsertErr error
	deleteErr error
	loadErr   error
}

func (f *fakeSessionsStore) Load(_ string) ([]domain.Session, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	out := make([]domain.Session, len(f.sessions))
	copy(out, f.sessions)
	return out, nil
}

func (f *fakeSessionsStore) LoadFiltered(_ string, keep func(domain.Session) bool) ([]domain.Session, error) {
	if f.loadErr != nil {
		return nil, f.loadErr
	}
	var out []domain.Session
	for _, s := range f.sessions {
		if keep(s) {
			out = append(out, s)
		}
	}
	return out, nil
}

func (f *fakeSessionsStore) Upsert(s domain.Session) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append(f.upserted, s)
	for i := range f.sessions {
		if f.sessions[i].ID == s.ID {
			f.sessions[i] = s
			return nil
		}
	}
	f.sessions = append(f.sessions, s)
	return nil
}

func (f *fakeSessionsStore) UpsertBatch(sessions []domain.Session) error {
	for _, s := range sessions {
		if err := f.Upsert(s); err != nil {
			return err
		}
	}
	return nil
}

func (f *fakeSessionsStore) Delete(_ string, id string) error {
	if f.deleteErr != nil {
		return f.deleteErr
	}
	f.deleted = append(f.deleted, id)
	out := f.sessions[:0]
	for _, s := range f.sessions {
		if s.ID != id {
			out = append(out, s)
		}
	}
	f.sessions = out
	return nil
}

func (f *fakeSessionsStore) Append(s domain.Session) error {
	f.sessions = append(f.sessions, s)
	return nil
}

func (f *fakeSessionsStore) AppendBatch(sessions []domain.Session) error {
	f.sessions = append(f.sessions, sessions...)
	return nil
}

func (f *fakeSessionsStore) Rewrite(sessions []domain.Session) error {
	f.sessions = make([]domain.Session, len(sessions))
	copy(f.sessions, sessions)
	return nil
}

// helper: construct a Sessions use case backed by the given fakes.
func mkSessions(projects *fakeSessionsProjectStore, sessions *fakeSessionsStore) *usecase.Sessions {
	return usecase.NewSessions(nil, projects, sessions, nil)
}

// ---- ResolveProject ----

// Branch 1a: explicitID resolves to an existing project.
func TestUnit_Sessions_ResolveProject_ExplicitIDFound(t *testing.T) {
	t.Parallel()
	proj := domain.Project{ID: "p1", Name: "Alpha", Slug: "alpha"}
	ps := &fakeSessionsProjectStore{projects: []domain.Project{proj}}
	uc := mkSessions(ps, &fakeSessionsStore{})

	got, err := uc.ResolveProject("u1", "p1", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "p1" {
		t.Errorf("expected ID %q, got %q", "p1", got.ID)
	}
}

// Branch 1b: explicitID is given but not found → ErrProjectNotFound.
func TestUnit_Sessions_ResolveProject_ExplicitIDNotFound(t *testing.T) {
	t.Parallel()
	ps := &fakeSessionsProjectStore{}
	uc := mkSessions(ps, &fakeSessionsStore{})

	_, err := uc.ResolveProject("u1", "missing-id", "")
	if !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

// Branch 2: pwd basename matches an existing slug.
func TestUnit_Sessions_ResolveProject_PWDBasenameMatch(t *testing.T) {
	t.Parallel()
	// basename("/home/user/my-project") → "my-project" → slug "my-project"
	proj := domain.Project{ID: "p2", Name: "My Project", Slug: "my-project"}
	ps := &fakeSessionsProjectStore{projects: []domain.Project{proj}}
	uc := mkSessions(ps, &fakeSessionsStore{})

	got, err := uc.ResolveProject("u1", "", "/home/user/my-project")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "p2" {
		t.Errorf("expected ID %q, got %q", "p2", got.ID)
	}
}

// Branch 3: pwd provided but slug does not match; ListActive has entries
// → returns ListActive[0] (MRU first).
func TestUnit_Sessions_ResolveProject_MRUFallback(t *testing.T) {
	t.Parallel()
	// slug of "unknown-dir" is "unknown-dir" — not in store.
	mru := domain.Project{ID: "p-mru", Name: "Recent", Slug: "recent"}
	other := domain.Project{ID: "p-other", Name: "Other", Slug: "other"}
	ps := &fakeSessionsProjectStore{projects: []domain.Project{mru, other}}
	uc := mkSessions(ps, &fakeSessionsStore{})

	got, err := uc.ResolveProject("u1", "", "/home/user/unknown-dir")
	if err != nil {
		t.Fatal(err)
	}
	if got.ID != "p-mru" {
		t.Errorf("expected MRU project %q, got %q", "p-mru", got.ID)
	}
}

// Branch 4: pwd provided but no slug match, ListActive is empty
// → EnsureBySlug("Allgemein", "allgemein") is called and returned.
func TestUnit_Sessions_ResolveProject_AllgemeinFallback(t *testing.T) {
	t.Parallel()
	// empty store → no slug match, no active projects.
	ps := &fakeSessionsProjectStore{}
	uc := mkSessions(ps, &fakeSessionsStore{})

	got, err := uc.ResolveProject("u1", "", "/home/user/unknown-dir")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "allgemein" {
		t.Errorf("expected slug %q, got %q", "allgemein", got.Slug)
	}
	if got.Name != "Allgemein" {
		t.Errorf("expected name %q, got %q", "Allgemein", got.Name)
	}
}

// Branch 4 with empty pwd (no env): step 2 is skipped, still falls through
// to Allgemein when ListActive is empty.
func TestUnit_Sessions_ResolveProject_EmptyPWD_AllgemeinFallback(t *testing.T) {
	t.Parallel()
	ps := &fakeSessionsProjectStore{}
	uc := mkSessions(ps, &fakeSessionsStore{})

	got, err := uc.ResolveProject("u1", "", "")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "allgemein" {
		t.Errorf("expected slug %q, got %q", "allgemein", got.Slug)
	}
}

// ---- Edit ----

// Edit mutates a copy, bumps Version, and calls Upsert with the updated session.
func TestUnit_Sessions_Edit_MutatesAndBumpsVersion(t *testing.T) {
	t.Parallel()
	existing := domain.Session{
		ID:      "s1",
		UserID:  "u1",
		Tag:     "deep",
		Version: 2,
	}
	ss := &fakeSessionsStore{sessions: []domain.Session{existing}}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Edit("u1", "s1", func(s *domain.Session) {
		s.Tag = "meeting"
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(ss.upserted) != 1 {
		t.Fatalf("expected 1 upsert, got %d", len(ss.upserted))
	}
	up := ss.upserted[0]
	if up.Tag != "meeting" {
		t.Errorf("expected Tag %q, got %q", "meeting", up.Tag)
	}
	if up.Version != 3 {
		t.Errorf("expected Version=3 (bumped), got %d", up.Version)
	}
}

// Edit with unknown ID returns ErrSessionNotFound.
func TestUnit_Sessions_Edit_NotFound(t *testing.T) {
	t.Parallel()
	ss := &fakeSessionsStore{}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Edit("u1", "no-such-id", func(_ *domain.Session) {})
	if !errors.Is(err, usecase.ErrSessionNotFound) {
		t.Errorf("expected ErrSessionNotFound, got %v", err)
	}
}

// Edit propagates LoadFiltered errors.
func TestUnit_Sessions_Edit_PropagatesLoadError(t *testing.T) {
	t.Parallel()
	ss := &fakeSessionsStore{loadErr: errors.New("disk error")}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Edit("u1", "s1", func(_ *domain.Session) {})
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// Edit propagates Upsert errors (including ErrSessionVersionConflict).
func TestUnit_Sessions_Edit_PropagatesUpsertError(t *testing.T) {
	t.Parallel()
	existing := domain.Session{ID: "s1", UserID: "u1", Version: 1}
	ss := &fakeSessionsStore{
		sessions:  []domain.Session{existing},
		upsertErr: ports.ErrSessionVersionConflict,
	}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Edit("u1", "s1", func(_ *domain.Session) {})
	if !errors.Is(err, ports.ErrSessionVersionConflict) {
		t.Errorf("expected ErrSessionVersionConflict, got %v", err)
	}
}

// ---- Delete ----

// Delete delegates to SessionStore.Delete.
func TestUnit_Sessions_Delete_Delegates(t *testing.T) {
	t.Parallel()
	existing := domain.Session{ID: "s1", UserID: "u1"}
	ss := &fakeSessionsStore{sessions: []domain.Session{existing}}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Delete("u1", "s1")
	if err != nil {
		t.Fatal(err)
	}
	if len(ss.deleted) != 1 || ss.deleted[0] != "s1" {
		t.Errorf("expected Delete(s1), got %v", ss.deleted)
	}
}

// Delete propagates errors from SessionStore.Delete.
func TestUnit_Sessions_Delete_PropagatesError(t *testing.T) {
	t.Parallel()
	ss := &fakeSessionsStore{deleteErr: errors.New("constraint violation")}
	uc := mkSessions(&fakeSessionsProjectStore{}, ss)

	err := uc.Delete("u1", "s1")
	if err == nil {
		t.Error("expected error, got nil")
	}
}
