package daydetail

import (
	"context"
	"time"

	"github.com/serverkraken/flow/internal/domain"
)

// API is the daydetail route's view of the backend. Extended in later tasks
// (Tasks 6/7 add CreateSession, EditSession, DeleteSession, ListProjects).
type API interface {
	ListSessionsRange(ctx context.Context, since, until time.Time) ([]domain.WorkSession, error)
	AddSession(ctx context.Context, projectID *string, start, stop time.Time, tag, note string) (domain.WorkSession, error)
	ListProjects(ctx context.Context) ([]domain.Project, error)
	CreateProject(ctx context.Context, name string) (domain.Project, error)
}
