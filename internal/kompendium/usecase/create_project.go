package usecase

import (
	"context"
	"fmt"

	"github.com/serverkraken/flow/internal/kompendium/domain"
	"github.com/serverkraken/flow/internal/kompendium/ports"
)

// CreateProject creates a project note for today under
// "projects/<canonical-url>/<date>" and opens it in the editor. The
// canonical URL comes from RepoDetector applied to the input cwd.
type CreateProject struct {
	Store  ports.NoteStore
	Repo   ports.RepoDetector
	Clock  ports.Clock
	Editor ports.Editor
}

// NewCreateProject wires the use case with its required ports.
func NewCreateProject(
	store ports.NoteStore,
	repo ports.RepoDetector,
	clock ports.Clock,
	editor ports.Editor,
) *CreateProject {
	return &CreateProject{Store: store, Repo: repo, Clock: clock, Editor: editor}
}

// CreateProjectInput identifies the working directory whose enclosing repo
// the project note belongs to.
type CreateProjectInput struct {
	Cwd string
}

// CreateProjectOutput mirrors CreateDailyOutput.
type CreateProjectOutput struct {
	ID      domain.ID
	Project domain.CanonicalURL
	Created bool
}

// Execute resolves cwd to a repo, computes the project note ID, ensures the
// note exists, and opens it via a tempfile (see EditNote).
// ports.ErrNotInRepo from the detector propagates unchanged so callers can
// render a sensible message.
func (u *CreateProject) Execute(ctx context.Context, in CreateProjectInput) (CreateProjectOutput, error) {
	info, err := u.Repo.Detect(ctx, in.Cwd)
	if err != nil {
		return CreateProjectOutput{}, err
	}

	// Local date — see the rationale in CreateDaily.
	date := u.Clock.Now().Format("2006-01-02")
	id := domain.ID("projects/" + string(info.URL) + "/" + date)

	exists, err := u.Store.Exists(ctx, id)
	if err != nil {
		return CreateProjectOutput{}, fmt.Errorf("exists: %w", err)
	}

	if !exists {
		note, err := buildProjectTemplate(id, date, info.URL)
		if err != nil {
			return CreateProjectOutput{}, err
		}
		if err := u.Store.Put(ctx, note); err != nil {
			return CreateProjectOutput{}, fmt.Errorf("put: %w", err)
		}
	}

	edit := EditNote{Store: u.Store, Editor: u.Editor}
	if err := edit.Execute(ctx, id); err != nil {
		return CreateProjectOutput{}, fmt.Errorf("edit: %w", err)
	}
	return CreateProjectOutput{ID: id, Project: info.URL, Created: !exists}, nil
}

func buildProjectTemplate(id domain.ID, date string, url domain.CanonicalURL) (domain.Note, error) {
	return domain.NewNote(id, domain.Frontmatter{
		ID:      id.String(),
		Type:    domain.TypeProject,
		Project: string(url),
		Date:    date,
	}, []byte{})
}
