package cli

// Unit tests for resolveProjectRef (slug vs UUID disambiguation) and
// cobra command shapes for the CRUD subcommands (list/create/rename/archive).

import (
	"bytes"
	"errors"
	"strings"
	"testing"
	"time"

	tea "charm.land/bubbletea/v2"
	"github.com/serverkraken/flow/internal/domain"
	tk "github.com/serverkraken/flow/internal/frontend/tui/theme"
	"github.com/serverkraken/flow/internal/ports"
	"github.com/serverkraken/flow/internal/usecase"
)

// — fakeProjectStore —

// fakeProjectStore is an in-process ports.ProjectStore for unit tests.
// Only the methods exercised by resolveProjectRef and CRUD commands are
// implemented; all others panic to catch accidental calls.
type fakeProjectStore struct {
	byID     map[string]domain.Project
	bySlug   map[string]domain.Project
	upserted []domain.Project
	archived []string
}

var _ ports.ProjectStore = (*fakeProjectStore)(nil)

func newFakeProjectStore(projects ...domain.Project) *fakeProjectStore {
	f := &fakeProjectStore{
		byID:   make(map[string]domain.Project),
		bySlug: make(map[string]domain.Project),
	}
	for _, p := range projects {
		f.byID[p.ID] = p
		f.bySlug[p.Slug] = p
	}
	return f
}

func (f *fakeProjectStore) GetByID(_ string, id string) (domain.Project, error) {
	if p, ok := f.byID[id]; ok {
		return p, nil
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeProjectStore) GetBySlug(_ string, slug string) (domain.Project, error) {
	if p, ok := f.bySlug[slug]; ok {
		return p, nil
	}
	return domain.Project{}, ports.ErrProjectNotFound
}

func (f *fakeProjectStore) ListActive(_ string) ([]domain.Project, error) {
	var out []domain.Project
	for _, p := range f.byID {
		if p.ArchivedAt == nil {
			out = append(out, p)
		}
	}
	return out, nil
}

func (f *fakeProjectStore) ListAll(_ string) ([]domain.Project, error) {
	var out []domain.Project
	for _, p := range f.byID {
		out = append(out, p)
	}
	return out, nil
}

func (f *fakeProjectStore) EnsureBySlug(userID, name, slug string) (domain.Project, error) {
	if p, ok := f.bySlug[slug]; ok {
		return p, nil
	}
	p := domain.Project{
		ID:        "new-" + slug,
		UserID:    userID,
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Now(),
	}
	f.byID[p.ID] = p
	f.bySlug[slug] = p
	return p, nil
}

func (f *fakeProjectStore) Upsert(p domain.Project) error {
	f.byID[p.ID] = p
	f.bySlug[p.Slug] = p
	f.upserted = append(f.upserted, p)
	return nil
}

func (f *fakeProjectStore) TouchLastUsed(_ string, id string) error {
	p, ok := f.byID[id]
	if !ok {
		return ports.ErrProjectNotFound
	}
	now := time.Now()
	p.LastUsedAt = now
	f.byID[id] = p
	return nil
}

func (f *fakeProjectStore) Archive(_ string, id string) error {
	p, ok := f.byID[id]
	if !ok {
		return ports.ErrProjectNotFound
	}
	now := time.Now()
	p.ArchivedAt = &now
	f.byID[id] = p
	f.archived = append(f.archived, id)
	return nil
}

// — helper —

func testProject(id, slug, name string) domain.Project {
	return domain.Project{
		ID:        id,
		UserID:    "u1",
		Name:      name,
		Slug:      slug,
		CreatedAt: time.Date(2026, 1, 1, 0, 0, 0, 0, time.UTC),
	}
}

// — resolveProjectRef —

func TestResolveProjectRef_BySlug(t *testing.T) {
	t.Parallel()
	proj := testProject("aaa00000-0000-0000-0000-000000000001", "my-proj", "My Project")
	store := newFakeProjectStore(proj)
	got, err := resolveProjectRef(store, "u1", "my-proj")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != proj.ID {
		t.Errorf("got ID %q, want %q", got.ID, proj.ID)
	}
}

func TestResolveProjectRef_ByUUID(t *testing.T) {
	t.Parallel()
	proj := testProject("aaa00000-0000-0000-0000-000000000001", "my-proj", "My Project")
	store := newFakeProjectStore(proj)
	got, err := resolveProjectRef(store, "u1", "aaa00000-0000-0000-0000-000000000001")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.Slug != proj.Slug {
		t.Errorf("got slug %q, want %q", got.Slug, proj.Slug)
	}
}

func TestResolveProjectRef_SlugNotFound(t *testing.T) {
	t.Parallel()
	store := newFakeProjectStore()
	_, err := resolveProjectRef(store, "u1", "no-such-slug")
	if !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

func TestResolveProjectRef_UUIDNotFound(t *testing.T) {
	t.Parallel()
	store := newFakeProjectStore()
	_, err := resolveProjectRef(store, "u1", "00000000-0000-0000-0000-000000000099")
	if !errors.Is(err, ports.ErrProjectNotFound) {
		t.Errorf("expected ErrProjectNotFound, got %v", err)
	}
}

// Strings that look like partial UUIDs but are not valid — treated as slug.
func TestResolveProjectRef_PartialUUIDTreatedAsSlug(t *testing.T) {
	t.Parallel()
	proj := testProject("x", "ab12cd34-ef56", "Weird Slug") // slug happens to look like part of a UUID
	store := newFakeProjectStore(proj)
	got, err := resolveProjectRef(store, "u1", "ab12cd34-ef56")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if got.ID != "x" {
		t.Errorf("expected slug lookup, got ID %q", got.ID)
	}
}

// — cobra shape tests —

func makeCRUDDeps() (ProjectsCRUDDeps, *fakeProjectStore) {
	proj := testProject("aaa00000-0000-0000-0000-000000000001", "existing-proj", "Existing Project")
	store := newFakeProjectStore(proj)
	uc := usecase.NewProjects(&fakeUserStore{}, store)
	return ProjectsCRUDDeps{Projects: uc, UserID: "u1"}, store
}

func TestNewProjectsListCmd_Shape(t *testing.T) {
	t.Parallel()
	deps, _ := makeCRUDDeps()
	cmd := newProjectsListCmd(deps)
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "list" {
		t.Errorf("Use: %q, want list", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
	if f := cmd.Flags().Lookup("archived"); f == nil {
		t.Errorf("--archived flag missing")
	}
}

func TestNewProjectsCreateCmd_Shape(t *testing.T) {
	t.Parallel()
	deps, _ := makeCRUDDeps()
	cmd := newProjectsCreateCmd(deps)
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "create <name>" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
}

func TestNewProjectsRenameCmd_Shape(t *testing.T) {
	t.Parallel()
	deps, store := makeCRUDDeps()
	cmd := newProjectsRenameCmd(deps, store)
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "rename <slug-or-id> <new-name>" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
}

func TestNewProjectsArchiveCmd_Shape(t *testing.T) {
	t.Parallel()
	deps, store := makeCRUDDeps()
	cmd := newProjectsArchiveCmd(deps, store)
	if cmd == nil {
		t.Fatal("nil command")
	}
	if cmd.Use != "archive <slug-or-id>" {
		t.Errorf("Use: %q", cmd.Use)
	}
	if !cmd.SilenceUsage {
		t.Errorf("SilenceUsage must be true")
	}
	if cmd.RunE == nil {
		t.Errorf("RunE must be set")
	}
}

// — RunE behaviour —

func TestProjectsListCmd_OutputsHeader(t *testing.T) {
	t.Parallel()
	deps, _ := makeCRUDDeps()
	cmd := newProjectsListCmd(deps)
	var buf bytes.Buffer
	cmd.SetOut(&buf)
	cmd.SetErr(&buf)
	if err := cmd.RunE(cmd, nil); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if !strings.Contains(buf.String(), "SLUG") {
		t.Errorf("list output missing header, got:\n%s", buf.String())
	}
	if !strings.Contains(buf.String(), "existing-proj") {
		t.Errorf("list output missing project slug, got:\n%s", buf.String())
	}
}

func TestProjectsCreateCmd_PrintsSlug(t *testing.T) {
	t.Parallel()
	deps, _ := makeCRUDDeps()
	cmd := newProjectsCreateCmd(deps)
	var out bytes.Buffer
	cmd.SetOut(&out)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.RunE(cmd, []string{"New Project"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	slug := strings.TrimSpace(out.String())
	if slug != "new-project" {
		t.Errorf("printed slug %q, want new-project", slug)
	}
}

func TestProjectsRenameCmd_RenamesProject(t *testing.T) {
	t.Parallel()
	deps, store := makeCRUDDeps()
	cmd := newProjectsRenameCmd(deps, store)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.RunE(cmd, []string{"existing-proj", "Renamed Project"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	// Verify upserted project has new name.
	if len(store.upserted) == 0 {
		t.Fatalf("nothing upserted")
	}
	found := false
	for _, p := range store.upserted {
		if p.Name == "Renamed Project" {
			found = true
		}
	}
	if !found {
		t.Errorf("renamed project not upserted with new name; upserted: %+v", store.upserted)
	}
}

func TestProjectsArchiveCmd_ArchivesProject(t *testing.T) {
	t.Parallel()
	deps, store := makeCRUDDeps()
	cmd := newProjectsArchiveCmd(deps, store)
	cmd.SetErr(&bytes.Buffer{})
	if err := cmd.RunE(cmd, []string{"existing-proj"}); err != nil {
		t.Fatalf("RunE: %v", err)
	}
	if len(store.archived) == 0 {
		t.Fatalf("nothing archived")
	}
	if store.archived[0] != "aaa00000-0000-0000-0000-000000000001" {
		t.Errorf("archived ID %q, want the project's ID", store.archived[0])
	}
}

// — fakeUserStore — (minimal, satisfies ports.UserStore for usecase.NewProjects)

type fakeUserStore struct{}

var _ ports.UserStore = (*fakeUserStore)(nil)

func (f *fakeUserStore) EnsureBySub(sub, _, _ string) (domain.User, error) {
	return domain.User{ID: sub, OIDCSub: sub}, nil
}

func (f *fakeUserStore) GetByID(id string) (domain.User, error) {
	return domain.User{ID: id}, nil
}

func (f *fakeUserStore) GetBySub(sub string) (domain.User, error) {
	return domain.User{ID: sub, OIDCSub: sub}, nil
}

// — NewProjectsCmd with CRUD registered —

func TestNewProjectsCmd_WithCRUD_HasSubcommands(t *testing.T) {
	t.Parallel()
	proj := testProject("aaa00000-0000-0000-0000-000000000001", "x", "X")
	store := newFakeProjectStore(proj)
	uc := usecase.NewProjects(&fakeUserStore{}, store)
	cruddeps := &ProjectsCRUDDeps{Projects: uc, UserID: "u1"}
	cmd := NewProjectsCmd(ProjectsDeps{
		Screen:       func(tk.Palette) tea.Model { return stubScreen{} },
		CRUD:         cruddeps,
		ProjectStore: store,
	})

	names := make(map[string]bool)
	for _, sub := range cmd.Commands() {
		names[sub.Name()] = true
	}
	for _, want := range []string{"list", "create", "rename", "archive"} {
		if !names[want] {
			t.Errorf("missing subcommand %q; found: %v", want, names)
		}
	}
}

func TestNewProjectsCmd_WithoutCRUD_NoSubcommands(t *testing.T) {
	t.Parallel()
	cmd := NewProjectsCmd(ProjectsDeps{
		Screen: func(tk.Palette) tea.Model { return stubScreen{} },
		CRUD:   nil,
	})
	if len(cmd.Commands()) != 0 {
		t.Errorf("expected no subcommands when CRUD is nil, got %d", len(cmd.Commands()))
	}
}
