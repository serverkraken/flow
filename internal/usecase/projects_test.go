package usecase_test

import (
	"errors"
	"testing"
	"time"

	"github.com/serverkraken/flow/internal/domain"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// fakeProjectStore is a hand-rolled in-memory double for ports.ProjectStore.
type fakeProjectStore struct {
	projects   []domain.Project
	listErr    error
	getByIDErr error
	getBySlug  func(userID, slug string) (domain.Project, error)
	ensureErr  error
	upsertErr  error
	touchErr   error
	archiveErr error
	archived   []string // IDs that received Archive
	touched    []string // IDs that received TouchLastUsed
	upserted   []domain.Project
}

func (f *fakeProjectStore) ListActive(_ string) ([]domain.Project, error) {
	if f.listErr != nil {
		return nil, f.listErr
	}
	return f.projects, nil
}

func (f *fakeProjectStore) ListAll(_ string) ([]domain.Project, error) {
	return f.projects, nil
}

func (f *fakeProjectStore) GetByID(_, id string) (domain.Project, error) {
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

func (f *fakeProjectStore) GetBySlug(userID, slug string) (domain.Project, error) {
	if f.getBySlug != nil {
		return f.getBySlug(userID, slug)
	}
	for _, p := range f.projects {
		if p.Slug == slug {
			return p, nil
		}
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeProjectStore) EnsureBySlug(_, name, slug string) (domain.Project, error) {
	if f.ensureErr != nil {
		return domain.Project{}, f.ensureErr
	}
	p := domain.Project{
		ID:        "new-id",
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now(),
	}
	f.projects = append(f.projects, p)
	return p, nil
}

func (f *fakeProjectStore) Upsert(p domain.Project) error {
	if f.upsertErr != nil {
		return f.upsertErr
	}
	f.upserted = append(f.upserted, p)
	for i := range f.projects {
		if f.projects[i].ID == p.ID {
			f.projects[i] = p
			return nil
		}
	}
	f.projects = append(f.projects, p)
	return nil
}

func (f *fakeProjectStore) TouchLastUsed(_, id string) error {
	if f.touchErr != nil {
		return f.touchErr
	}
	f.touched = append(f.touched, id)
	return nil
}

func (f *fakeProjectStore) Archive(_, id string) error {
	if f.archiveErr != nil {
		return f.archiveErr
	}
	f.archived = append(f.archived, id)
	return nil
}

// --- SlugFromName ---

func TestUnit_SlugFromName(t *testing.T) {
	t.Parallel()
	cases := []struct {
		input string
		want  string
	}{
		{"Mein Projekt", "mein-projekt"},
		{"Flow!", "flow"},
		{"---", "unnamed"},
		{"Über", "ber"},
		{"", "unnamed"},
		{"  leading and trailing  ", "leading-and-trailing"},
		{"Multiple   Spaces", "multiple-spaces"},
		{"hello_world", "hello-world"},
		{"kebab-case", "kebab-case"},
		{"abc123", "abc123"},
		{"!@#$%", "unnamed"},
		{"Go Projekt 2025", "go-projekt-2025"},
	}
	for _, tc := range cases {
		tc := tc
		t.Run(tc.input, func(t *testing.T) {
			t.Parallel()
			got := usecase.SlugFromName(tc.input)
			if got != tc.want {
				t.Errorf("SlugFromName(%q) = %q, want %q", tc.input, got, tc.want)
			}
		})
	}
}

// --- ListActive ---

func TestUnit_Projects_ListActive_Delegates(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{
		projects: []domain.Project{
			{ID: "1", Name: "Alpha", Slug: "alpha"},
			{ID: "2", Name: "Beta", Slug: "beta"},
		},
	}
	uc := usecase.NewProjects(nil, store)
	got, err := uc.ListActive("user1")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 2 {
		t.Errorf("expected 2 projects, got %d", len(got))
	}
}

func TestUnit_Projects_ListActive_PropagatesError(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{listErr: errors.New("db down")}
	uc := usecase.NewProjects(nil, store)
	_, err := uc.ListActive("user1")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

// --- Create ---

func TestUnit_Projects_Create_EmptyName(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{}
	uc := usecase.NewProjects(nil, store)
	_, err := uc.Create("user1", "")
	if err == nil {
		t.Error("expected error for empty name, got nil")
	}
}

func TestUnit_Projects_Create_WhitespaceName(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{}
	uc := usecase.NewProjects(nil, store)
	_, err := uc.Create("user1", "   ")
	if err == nil {
		t.Error("expected error for whitespace-only name, got nil")
	}
}

func TestUnit_Projects_Create_NoCollision(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{}
	uc := usecase.NewProjects(nil, store)
	got, err := uc.Create("user1", "Mein Projekt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "mein-projekt" {
		t.Errorf("expected slug %q, got %q", "mein-projekt", got.Slug)
	}
}

func TestUnit_Projects_Create_SlugCollisionSuffix(t *testing.T) {
	t.Parallel()
	// First call to GetBySlug("mein-projekt") finds an existing project.
	// Second call to GetBySlug("mein-projekt-2") returns not-found.
	callCount := 0
	store := &fakeProjectStore{
		getBySlug: func(_, slug string) (domain.Project, error) {
			callCount++
			if slug == "mein-projekt" {
				return domain.Project{ID: "existing", Slug: "mein-projekt"}, nil
			}
			return domain.Project{}, ports.ErrProjectNotFound
		},
	}
	uc := usecase.NewProjects(nil, store)
	got, err := uc.Create("user1", "Mein Projekt")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "mein-projekt-2" {
		t.Errorf("expected slug %q, got %q", "mein-projekt-2", got.Slug)
	}
}

func TestUnit_Projects_Create_SlugCollisionMultipleSuffixes(t *testing.T) {
	t.Parallel()
	// "flow" and "flow-2" both exist; "flow-3" is free.
	store := &fakeProjectStore{
		getBySlug: func(_, slug string) (domain.Project, error) {
			switch slug {
			case "flow", "flow-2":
				return domain.Project{Slug: slug}, nil
			default:
				return domain.Project{}, ports.ErrProjectNotFound
			}
		},
	}
	uc := usecase.NewProjects(nil, store)
	got, err := uc.Create("user1", "Flow")
	if err != nil {
		t.Fatal(err)
	}
	if got.Slug != "flow-3" {
		t.Errorf("expected slug %q, got %q", "flow-3", got.Slug)
	}
}

// --- Rename ---

func TestUnit_Projects_Rename_KeepsSlugStable(t *testing.T) {
	t.Parallel()
	existing := domain.Project{
		ID:      "proj-1",
		UserID:  "user1",
		Name:    "Old Name",
		Slug:    "old-name",
		Version: 0,
	}
	store := &fakeProjectStore{projects: []domain.Project{existing}}
	uc := usecase.NewProjects(nil, store)

	err := uc.Rename("user1", "proj-1", "New Name")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.upserted) != 1 {
		t.Fatalf("expected 1 upsert call, got %d", len(store.upserted))
	}
	up := store.upserted[0]
	if up.Slug != "old-name" {
		t.Errorf("Rename changed slug: got %q, want %q", up.Slug, "old-name")
	}
	if up.Name != "New Name" {
		t.Errorf("Rename did not update name: got %q", up.Name)
	}
}

func TestUnit_Projects_Rename_BumpsVersion(t *testing.T) {
	t.Parallel()
	existing := domain.Project{
		ID:      "proj-1",
		Name:    "Alpha",
		Slug:    "alpha",
		Version: 3,
	}
	store := &fakeProjectStore{projects: []domain.Project{existing}}
	uc := usecase.NewProjects(nil, store)

	if err := uc.Rename("user1", "proj-1", "Alpha Updated"); err != nil {
		t.Fatal(err)
	}
	if store.upserted[0].Version != 4 {
		t.Errorf("expected Version=4, got %d", store.upserted[0].Version)
	}
}

func TestUnit_Projects_Rename_PropagatesGetError(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{getByIDErr: errors.New("not found")}
	uc := usecase.NewProjects(nil, store)
	err := uc.Rename("user1", "missing", "Whatever")
	if err == nil {
		t.Error("expected error, got nil")
	}
}

func TestUnit_Projects_Rename_PropagatesUpsertError(t *testing.T) {
	t.Parallel()
	existing := domain.Project{ID: "proj-1", Name: "X", Slug: "x"}
	store := &fakeProjectStore{
		projects:  []domain.Project{existing},
		upsertErr: errors.New("disk full"),
	}
	uc := usecase.NewProjects(nil, store)
	if err := uc.Rename("user1", "proj-1", "Y"); err == nil {
		t.Error("expected error, got nil")
	}
}

// --- Archive ---

func TestUnit_Projects_Archive_Delegates(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{}
	uc := usecase.NewProjects(nil, store)
	err := uc.Archive("user1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.archived) != 1 || store.archived[0] != "proj-1" {
		t.Errorf("expected Archive(%q), got %v", "proj-1", store.archived)
	}
}

func TestUnit_Projects_Archive_PropagatesError(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{archiveErr: errors.New("boom")}
	uc := usecase.NewProjects(nil, store)
	if err := uc.Archive("user1", "proj-1"); err == nil {
		t.Error("expected error, got nil")
	}
}

// --- MarkUsedNow ---

func TestUnit_Projects_MarkUsedNow_Delegates(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{}
	uc := usecase.NewProjects(nil, store)
	err := uc.MarkUsedNow("user1", "proj-1")
	if err != nil {
		t.Fatal(err)
	}
	if len(store.touched) != 1 || store.touched[0] != "proj-1" {
		t.Errorf("expected TouchLastUsed(%q), got %v", "proj-1", store.touched)
	}
}

func TestUnit_Projects_MarkUsedNow_PropagatesError(t *testing.T) {
	t.Parallel()
	store := &fakeProjectStore{touchErr: errors.New("boom")}
	uc := usecase.NewProjects(nil, store)
	if err := uc.MarkUsedNow("user1", "proj-1"); err == nil {
		t.Error("expected error, got nil")
	}
}
